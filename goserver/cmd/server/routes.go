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
	r.Post("/api/v1/restart/container", app.RestartIPSecContainer)
	r.Post("/api/v1/restart/service", app.RestartIPSecService)
	r.Post("/api/v1/exec", app.ExecCommandInContainer)
	r.Get("/api/v1/version", app.HandleVersion)

	return r
}
