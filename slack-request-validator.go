package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	slackSignatureSHA256Header  = "X-Slack-Signature"
	slackRequestTimestampHeader = "X-Slack-Request-Timestamp"
)

type SlackRequestValidator struct {
	Secret []byte
}

func (v *SlackRequestValidator) validate(req *http.Request) error {
	signatureWithPrefix := req.Header.Get(slackSignatureSHA256Header)
	if len(signatureWithPrefix) == 0 {
		return fmt.Errorf("Missing \"%s\" header", slackSignatureSHA256Header)
	}

	splitSignature := strings.SplitN(signatureWithPrefix, "=", 2)
	if len(splitSignature) != 2 {
		return fmt.Errorf("Invalid signature format \"%s\"", signatureWithPrefix)
	}
	version := splitSignature[0]
	signature := splitSignature[1]

	timestampString := req.Header.Get(slackRequestTimestampHeader)
	if len(timestampString) == 0 {
		return fmt.Errorf("Missing \"%s\" header", slackRequestTimestampHeader)
	}

	timestampInt, err := strconv.ParseInt(timestampString, 10, 64)
	if err != nil {
		return fmt.Errorf("Invalid timestamp: %s", timestampString)
	}

	timestamp := time.Unix(timestampInt, 0)
	if timestamp.Before(time.Now().Add(-5 * time.Minute)) {
		return fmt.Errorf("Request is too old (timestamp = %s, now = %s)", timestamp.Format(time.RFC3339), time.Now().Format(time.RFC3339))
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return err
	}
	req.Body.Close()
	req.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	payload := fmt.Sprintf("%s:%s:%s", version, timestampString, body)
	actual := make([]byte, 32)
	hex.Decode(actual, []byte(signature))

	computed := hmac.New(sha256.New, v.Secret)
	computed.Write([]byte(payload))
	signed := []byte(computed.Sum(nil))

	if !hmac.Equal(signed, actual) {
		return fmt.Errorf("Invalid SHA256 signature")
	}

	return nil
}

func (v *SlackRequestValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := v.validate(r); err != nil {
			respondWithJSON(w, http.StatusBadRequest, err, "", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
