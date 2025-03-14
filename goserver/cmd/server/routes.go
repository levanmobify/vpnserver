package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (app *application) routes() *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(app.AuthMiddleware)

	r.Get("/api/v1/users", app.ListUsersHandler)
	r.Post("/api/v1/users", app.AddUserHandler)

	return r
}
