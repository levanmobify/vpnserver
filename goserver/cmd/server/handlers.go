package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/LevanPro/server/internal/models"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type ExecRequest struct {
	Container string   `json:"container"`
	Command   []string `json:"command"`
}

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

func (app *application) HandleVersion(w http.ResponseWriter, r *http.Request) {

	resp := make(map[string]int, 1)
	resp["version"] = 1

	err := app.writeJSON(w, http.StatusOK, envolope{
		"data": resp,
	}, nil)

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
			app.badRequestResponse(w, r, fmt.Errorf("user with that username already exists %s", user.Username))
			return
		}
	}

	psk, err := app.fileService.ReadPSKSecret()

	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	for i, _ := range users {
		users[i].PSKSecret = psk
		err := app.userService.AddPassword(&users[i])
		if err != nil {
			app.serverErrorResponse(w, r, err)
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

func (app *application) RestartIPSecContainer(w http.ResponseWriter, r *http.Request) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.47"))

	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	ctx := context.Background()
	timeout := 5

	err = cli.ContainerRestart(ctx, "ipsec-mobify-server", container.StopOptions{
		Timeout: &timeout,
	})

	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envolope{"message": "success restarting container"}, nil)

	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) RestartIPSecService(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.47"))
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	defer cli.Close()

	execConfig := container.ExecOptions{
		Cmd:          []string{"ipsec", "restart"},
		AttachStdout: true,
		AttachStderr: true,
		Privileged:   true,
	}

	execIDResp, err := cli.ContainerExecCreate(ctx, "ipsec-mobify-server", execConfig)
	if err != nil {
		app.serverErrorResponse(w, r, fmt.Errorf("failed to create exec: %v", err))
		return
	}

	resp, err := cli.ContainerExecAttach(ctx, execIDResp.ID, container.ExecAttachOptions{})
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	defer resp.Close()

	err = app.writeJSON(w, http.StatusOK, envolope{"message": "success restarting service"}, nil)

	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) ExecCommandInContainer(w http.ResponseWriter, r *http.Request) {
	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.badRequestResponse(w, r, errors.New("bad request"))
		return
	}

	result, err := app.dockerExec(req.Container, req.Command)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envolope{"data": result}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}

func (app *application) dockerExec(containerName string, cmd []string) (string, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}
	defer cli.Close()

	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := cli.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return "", err
	}

	resp, err := cli.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", err
	}
	defer resp.Close()

	outputBuf := new(bytes.Buffer)
	errorBuf := new(bytes.Buffer)

	_, err = stdcopy.StdCopy(outputBuf, errorBuf, resp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to decode docker stream: %v", err)
	}

	return outputBuf.String(), nil
}

func (app *application) BandwidthMetricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics, err := app.bandwidthService.GetMetrics()
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envolope{"data": metrics}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
}
