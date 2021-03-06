package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/acme/autocert"
	"makeshift.dev/event-tracker/slack"

	// _ "github.com/mattn/go-sqlite3"
	_ "github.com/go-sql-driver/mysql"
)

var (
	isValidEventType = map[string]bool{
		"DEPLOYMENT":   true,
		"MERGE":        true,
		"APP RELEASE":  true,
		"EXPERIMENT":   true,
		"OPS ACTIVITY": true,
	}
)

const (
	contentTypeHeader         = "Content-Type"
	applicationJSON           = "application/json"
	applicationFormURLEncoded = "application/x-www-form-urlencoded"
)

type Response struct {
	Error   string      `json:"error"`
	Status  int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type server struct {
	db                 *sql.DB
	DBUser             *string
	DBPassword         *string
	DBName             *string
	DBPort             *int
	HTTPPort           *int
	HTTPSPort          *int
	Domain             *string
	GitHubSecret       *string
	SlackSigningSecret *string
	router             *mux.Router
	SlackClient        *slack.Client
	SlackLogChannel    *string
	Location           *time.Location
}

func respondWithJSON(w http.ResponseWriter,
	status int,
	err error,
	message string,
	data interface{},
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	errorString := ""
	if err != nil {
		errorString = err.Error()
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")
	encoder.Encode(&Response{
		Error:   errorString,
		Status:  status,
		Message: message,
		Data:    data,
	})

	if err != nil {
		log.Printf("Response status: %d error: %s\n", status, err.Error())
	}
}

func (s *server) initDB() {
	var err error
	s.db, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(db:%d)/%s", *s.DBUser, *s.DBPassword, *s.DBPort, *s.DBName))
	if err != nil {
		log.Fatalln(err.Error())
	}

	statement := `
CREATE TABLE IF NOT EXISTS events (
	id BIGINT(20) UNSIGNED,
	event_type VARCHAR(20) NOT NULL,
	start_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	end_time TIMESTAMP NULL DEFAULT NULL,
	notes TEXT DEFAULT NULL,
	metadata JSON DEFAULT NULL,
	insert_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (id)
)
`
	if _, err := s.db.Exec(statement); err != nil {
		log.Fatalln(err)
	}
}

func verboseLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if req, err := httputil.DumpRequest(r, true); err == nil {
			log.Println(string(req))
		} else {
			log.Printf("Failed to dump request. error: %s\n", err.Error())
		}
		next.ServeHTTP(w, r)
	})
}

func combinedLogginMiddleware(next http.Handler) http.Handler {
	return handlers.CombinedLoggingHandler(os.Stdout, next)
}

func (s *server) initAPI() {
	s.router = mux.NewRouter()
	s.router.Use(combinedLogginMiddleware)

	s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// This is used by the load balancer health check.
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	api := s.router.PathPrefix("/api").Subrouter()
	apiV0 := api.PathPrefix("/v0").Subrouter()

	// Add your routes as needed
	apiV0.HandleFunc("/record", s.RecordHandler).
		Methods(http.MethodPost).
		Headers(contentTypeHeader, applicationJSON)

	// GitHub Webhook handler
	githubValidator := GitHubWebHookValidator{Secret: []byte(*s.GitHubSecret)}
	githubAPI := apiV0.PathPrefix("/github").Subrouter()
	githubAPI.Use(githubValidator.Middleware)
	githubAPI.HandleFunc("", s.PullRequestHandler).
		Methods(http.MethodPost).
		Headers(contentTypeHeader, applicationJSON).
		Headers(githubEventHeader, pullRequestEvent)
	githubAPI.HandleFunc("", s.PushHandler).
		Methods(http.MethodPost).
		Headers(contentTypeHeader, applicationJSON).
		Headers(githubEventHeader, pushEvent)

	githubAPI.HandleFunc("", func(w http.ResponseWriter, r *http.Request) {
		eventType := r.Header.Get(githubEventHeader)
		respondWithJSON(w, http.StatusOK, nil, fmt.Sprintf("GitHub event '%s' not yet handled", eventType), nil)
	}).
		Methods(http.MethodPost).
		Headers(contentTypeHeader, applicationJSON)

	// Slack slash-command handler
	slackValidator := SlackRequestValidator{Secret: []byte(*s.SlackSigningSecret)}
	slackAPI := apiV0.PathPrefix("/slack").Subrouter()
	slackAPI.Use(verboseLoggingMiddleware)
	slackAPI.Use(slackValidator.Middleware)
	slackAPI.HandleFunc("/command", s.SlackCommandHandler).
		Methods(http.MethodPost).
		Headers(contentTypeHeader, applicationFormURLEncoded)
	slackAPI.HandleFunc("/interaction", s.SlackInteractionHandler).
		Methods(http.MethodPost).
		Headers(contentTypeHeader, applicationFormURLEncoded)

}

func (s *server) ServeHTTPOnly() {
	log.Println("serving HTTP only")
	s.initDB()
	defer s.db.Close()
	s.initAPI()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *s.HTTPPort),
		Handler: s.router,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalln(err)
		}
	}()

	log.Println("server ready")

	c := make(chan os.Signal, 1)

	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(15*time.Second))
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	go httpServer.Shutdown(ctx)

	<-ctx.Done()

	log.Println("shutting down")
	os.Exit(0)
}

func (s *server) ServeHTTPAndHTTPS() {
	log.Println("serving HTTP only")
	s.initDB()
	defer s.db.Close()
	s.initAPI()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *s.HTTPPort),
		Handler: s.router,
	}

	httpsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *s.HTTPSPort),
		Handler: s.router,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalln(err)
		}
	}()

	go func() {
		if err := httpsServer.ListenAndServe(); err != nil {
			log.Fatalln(err)
		}
	}()

	log.Println("server ready")

	c := make(chan os.Signal, 1)

	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(15*time.Second))
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	go httpServer.Shutdown(ctx)
	go httpsServer.Shutdown(ctx)

	<-ctx.Done()

	log.Println("shutting down")
	os.Exit(0)
}

func (s *server) ServeWithAutocert() {
	log.Println("serving HTTPS using autocert")
	s.initDB()
	defer s.db.Close()
	s.initAPI()

	cacheDir := filepath.Join("/tmp/cert", *s.Domain)
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.Mkdir(cacheDir, 0777); err != nil {
			log.Fatalln(err)
		}
	} else if err := os.Chmod(cacheDir, 0777); err != nil {
		log.Fatalln(err)
	}

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(*s.Domain),
		Cache:      autocert.DirCache(cacheDir),
	}

	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		GetCertificate: certManager.GetCertificate,
	}

	httpsServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *s.HTTPSPort),
		TLSConfig:    cfg,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),

		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      s.router, // Pass our instance of gorilla/mux in.
	}

	httpServer := &http.Server{
		Handler: certManager.HTTPHandler(nil),
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalln(err)
		}
	}()

	go func() {
		if err := httpsServer.ListenAndServeTLS("", ""); err != nil {
			log.Fatalln(err)
		}
	}()

	log.Println("server ready")

	c := make(chan os.Signal, 1)

	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(15*time.Second))
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	go httpServer.Shutdown(ctx)
	go httpsServer.Shutdown(ctx)

	<-ctx.Done()

	log.Println("shutting down")
	os.Exit(0)
}

func (s *server) writeToDBAndLog(ctx context.Context, event *Event) error {
	if err := event.ValidateAndRectify(); err != nil {
		return err
	}

	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata to []byte")
	}

	if !event.DryRun {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO events (
	id,
	event_type,
	start_time,
	end_time,
	notes,
	metadata
) VALUES (
	?,
	?,
	?,
	?,
	?,
	?
)
`, event.ID, event.EventType, event.StartTime, event.EndTime, event.Notes, metadata)
		if err != nil {
			return err
		}
		if err := s.logToSlackChannel(event); err != nil {
			return err
		}
	} else {
		endTimeBytes, _ := event.EndTime.MarshalJSON()
		endTime, _ := strconv.Unquote(string(endTimeBytes))

		log.Printf(`
INSERT INTO events (
	id,
	event_type,
	start_time,
	end_time,
	notes,
	metadata
) VALUES (
	%d,
	'%s',
	'%s',
	'%s',
	'%s',
	'%s'
)
`, event.ID, event.EventType, event.StartTime.Format(time.RFC3339), endTime, event.Notes, metadata)
	}

	return nil
}

func (s *server) logToSlackChannel(event *Event) error {
	if len(*s.SlackLogChannel) == 0 {
		return nil
	}

	// So this is kind of bizzar.
	// Templating fails with an error like:
	// template: :3:26: executing "" at <.Metadata.repository.full_name>: can't evaluate field repository in type interface {}
	// if the Metadata field is a struct!!
	// Make it an interface, and it just works.
	// idk dude.
	metadataBytes, err := json.Marshal(event.Metadata)
	if err != nil {
		log.Printf("Failed to log event to Slack channel with error: %s\n", err.Error())
		return err
	}

	metadata := new(interface{})
	if err := json.Unmarshal(metadataBytes, metadata); err != nil {
		log.Fatal(err.Error())
	}
	event.Metadata = metadata

	request := slack.NewChatPostMessageRequest(*s.SlackLogChannel)
	buffer := &bytes.Buffer{}
	if err := slackTemplate.Execute(buffer, event); err != nil {
		log.Printf("Failed to log event to Slack channel with error: %s\n", err.Error())
		return err
	}

	request.Text = buffer.String()
	if _, err := s.SlackClient.ChatPostMessage(request); err != nil {
		log.Printf("Failed to log event to Slack channel with error: %s\n", err.Error())
		return err
	}

	return nil
}
func init() {
	rand.Seed(time.Now().Unix())
}

func main() {
	s := server{}
	s.Domain = flag.String("domain", "www.makeshift.dev", "domain for which a certificate should be obtained")
	s.DBUser = flag.String("db-user", "user", "username for database access")
	s.DBPassword = flag.String("db-password", "password", "password for database access")
	s.DBName = flag.String("db-name", "test", "name of database")
	s.GitHubSecret = flag.String("github-secret", "secret", "github webhook secret")
	s.SlackSigningSecret = flag.String("slack-signing-secret", "secret", "slack signing secret")
	slackOauthToken := flag.String("slack-oauth-token", "secret", "slack oath token")
	s.SlackLogChannel = flag.String("slack-log-channel", "channel", "slack log channel")
	s.DBPort = flag.Int("db-port", 3306, "database port number")
	s.HTTPPort = flag.Int("http-port", 80, "port on which HTTP should be served")
	s.HTTPSPort = flag.Int("https-port", 443, "port on which HTTPS should be served")
	useAutocert := flag.String("use-autocert", "false", "specify \"true\" or \"false\" to serve HTTPS using autocert")
	timeZone := flag.String("time-zone", "America/New_York", "time zone to use when logging to various sources")
	flag.Parse()

	location, err := time.LoadLocation(*timeZone)
	if err != nil {
		log.Fatalf("failed to load time zone with error: %s", err.Error())
	}

	s.Location = location
	s.SlackClient = slack.New(*slackOauthToken)

	if *useAutocert == "true" {
		s.ServeWithAutocert()
	} else {
		s.ServeHTTPAndHTTPS()
	}
}
