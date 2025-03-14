package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/LevanPro/server/internal/models"
)

func (app *application) ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := app.fileService.ReadFile()

	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envolope{"data": users}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) AddUserHandler(w http.ResponseWriter, r *http.Request) {
	existingUsers, err := app.fileService.ReadFile()

	if err != nil {
		app.badRequestResponse(w, r, errors.New("error reading existing users"))
		return
	}

	usersMap := make(map[string]bool)
	for _, existingUser := range existingUsers {
		usersMap[existingUser.Username] = true
	}

	var users []models.User

	if err := json.NewDecoder(r.Body).Decode(&users); err != nil {
		app.badRequestResponse(w, r, errors.New("invalid request body"))
		return
	}

	for _, user := range users {
		if user.Username == "" {
			app.badRequestResponse(w, r, errors.New("user username is not provided"))
			return
		}

		_, ok := usersMap[user.Username]
		if ok {
			app.badRequestResponse(w, r, errors.New("user with that username already exists"))
			return
		}
	}

	for i, _ := range users {
		err := app.userService.AddPassword(&users[i])
		if err != nil {
			app.badRequestResponse(w, r, errors.New("user with that username already exists"))
			return
		}
	}

	err = app.fileService.AddUsers(users)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envolope{"users": users}, nil)

	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
