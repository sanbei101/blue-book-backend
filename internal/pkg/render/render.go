package render

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"

	validator "github.com/kamalyes/go-argus"
	"github.com/phuslu/log"
)

var validate = validator.New()

func init() {
	validator.SetLocale("zh")
}

type Response[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

type ResponseWithoutData struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type errorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func Success[T any](w http.ResponseWriter, msg string, data T) {
	writeJSON(w, http.StatusOK, Response[T]{
		Code: http.StatusOK,
		Msg:  msg,
		Data: data,
	})
}

func SuccessNoData(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNoContent, ResponseWithoutData{
		Code: http.StatusNoContent,
		Msg:  msg,
	})
}

func Error(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorResponse{
		Code: code,
		Msg:  msg,
	})
}

func writeJSON(w http.ResponseWriter, code int, response any) {
	if code == http.StatusNoContent {
		w.WriteHeader(code)
		return
	}

	payload, err := json.Marshal(response)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, err := w.Write(payload); err != nil {
		log.Error().Err(err).Msg("Failed to write response")
	}
}

func ReadBody[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	return readBody[T](w, r, false)
}

func ReadOptionalBody[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	return readBody[T](w, r, true)
}

func readBody[T any](w http.ResponseWriter, r *http.Request, optional bool) (T, error) {
	var body T

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		Error(w, http.StatusBadRequest, "JSON 格式非法")
		return body, err
	}
	if optional && len(bytes.TrimSpace(payload)) == 0 {
		return body, nil
	}

	if err := json.Unmarshal(payload, &body); err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		Error(w, http.StatusBadRequest, "JSON 格式非法")
		return body, err
	}

	if err := validate.Struct(body); err != nil {
		errs := validator.TranslateValidationErrors(err, "zh")
		errorMsgs := make([]string, 0, len(errs))
		for i := range errs {
			errorMsgs = append(errorMsgs, errs[i].Field+": "+errs[i].Message)
		}
		fullErrorMsg := strings.Join(errorMsgs, "; ")
		Error(w, http.StatusBadRequest, fullErrorMsg)
		return body, err
	}

	return body, nil
}
