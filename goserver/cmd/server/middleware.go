package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func (app *application) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		if authHeader == "" {
			app.badRequestResponse(w, r, errors.New("auth token is not provided"))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			app.badRequestResponse(w, r, errors.New("token is not valid"))
			return
		}

		tokenString := parts[1]

		if tokenString != app.cfg.AuthPassword {
			app.logger.Info(fmt.Sprintf("provided token %s ", tokenString))
			app.badRequestResponse(w, r, errors.New("provided token is not valid"))
			return
		}

		next.ServeHTTP(w, r)
	})
}
