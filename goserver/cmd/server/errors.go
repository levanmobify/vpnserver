package main

import (
	"fmt"
	"net/http"
)

func (app *application) errorResponse(w http.ResponseWriter, r *http.Request, status int, message interface{}) {
	env := envolope{"error": message}

	err := app.writeJSON(w, status, env, nil)
	if err != nil {
		w.WriteHeader(500)
	}

}

func (app *application) serverErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error(fmt.Sprintf("error while serving response: %s", err.Error()))

	message := "the server encountered a problem and could not process your request"
	app.errorResponse(w, r, http.StatusInternalServerError, message)
}

func (app *application) badRequestResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.errorResponse(w, r, http.StatusBadRequest, err.Error())
}
