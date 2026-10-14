package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func (s *Server) httpCommand(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeCommand(w, http.StatusUnauthorized, reply{Error: "unauthorized"})
		return
	}
	app, admin, err := s.commands.authenticate(r.Context(), token)
	if err != nil {
		code := "unavailable"
		status := http.StatusServiceUnavailable
		if errors.Is(err, errBadToken) {
			code = "unauthorized"
			status = http.StatusUnauthorized
		}
		writeCommand(w, status, reply{Error: code})
		return
	}
	// Admin commands stay on the WebSocket. An administrator token is not an application credential.
	if admin {
		writeCommand(w, http.StatusUnauthorized, reply{Error: "unauthorized"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeCommand(w, http.StatusBadRequest, reply{Error: "bad json"})
		return
	}
	var command frame
	if json.Unmarshal(body, &command) != nil {
		writeCommand(w, http.StatusBadRequest, reply{Error: "bad json"})
		return
	}
	command.Token = ""
	res := s.commands.handle(r.Context(), &peer{app: app}, command)
	res.ID = command.ID
	writeCommand(w, http.StatusOK, res)
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" || strings.Contains(token, " ") {
		return "", false
	}
	return token, true
}

func writeCommand(w http.ResponseWriter, status int, value reply) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
