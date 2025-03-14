package main

import (
	"encoding/json"
	"net/http"
)

type envolope map[string]interface{}

func (app *application) writeJSON(w http.ResponseWriter, status int, data envolope, headers http.Header) error {
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	js = append(js, '\n')

	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)
	return nil
}
