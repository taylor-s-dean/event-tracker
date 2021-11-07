package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

const (
	signatureSHA1Header   = "X-Hub-Signature"
	signatureSHA256Header = "X-Hub-Signature-256"
	githubEventHeader     = "X-GitHub-Event"
	githubDeliverHeader   = "X-GitHub-Delivery"

	pullRequestEvent = "pull_request"
	pingEvent        = "ping"
)

var (
	validEvents = map[string]bool{
		pullRequestEvent: true,
		pingEvent:        true,
	}
)

type GitHubWebHookValidator struct {
	Secret []byte
}

func (v *GitHubWebHookValidator) verifySignatureSHA1(signature string, body []byte) bool {
	const signaturePrefix = "sha1="
	const signatureLength = 45 // len(SignaturePrefix) + len(hex(sha1))

	if len(signature) != signatureLength || !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}

	actual := make([]byte, 20)
	hex.Decode(actual, []byte(signature[5:]))

	computed := hmac.New(sha1.New, v.Secret)
	computed.Write(body)
	signed := []byte(computed.Sum(nil))

	return hmac.Equal(signed, actual)
}

func (v *GitHubWebHookValidator) verifySignatureSHA256(signature string, body []byte) bool {
	const signaturePrefix = "sha256="
	const signatureLength = 71 // len(SignaturePrefix) + len(hex(sha256))

	if len(signature) != signatureLength || !strings.HasPrefix(signature, signaturePrefix) {
		return false
	}

	actual := make([]byte, 32)
	hex.Decode(actual, []byte(signature[7:]))

	computed := hmac.New(sha256.New, v.Secret)
	computed.Write(body)
	signed := []byte(computed.Sum(nil))

	return hmac.Equal(signed, actual)
}

func (v *GitHubWebHookValidator) parseHook(req *http.Request) error {
	signatureSHA1 := req.Header.Get(signatureSHA1Header)
	signatureSHA256 := req.Header.Get(signatureSHA256Header)

	if len(signatureSHA1) == 0 {
		return fmt.Errorf("Missing \"%s\" header", signatureSHA1Header)
	}

	if len(signatureSHA256) == 0 {
		return fmt.Errorf("Missing \"%s\" header", signatureSHA256Header)
	}

	payload, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return err
	}
	req.Body.Close()
	req.Body = ioutil.NopCloser(bytes.NewBuffer(payload))

	if !v.verifySignatureSHA1(signatureSHA1, payload) {
		return errors.New("Invalid SHA1 signature")
	}

	if !v.verifySignatureSHA256(signatureSHA256, payload) {
		return errors.New("Invalid SHA256 signature")
	}

	githubEvent := req.Header.Get(githubEventHeader)
	if !validEvents[githubEvent] {
		log.Printf("GitHub event type \"%s\" not handled", githubEvent)
	}

	return nil
}

func (v *GitHubWebHookValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := v.parseHook(r); err != nil {
			respondWithJSON(w, http.StatusBadRequest, err, "", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
