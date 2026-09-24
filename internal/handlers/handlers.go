package handlers

import "net/http"

func HandlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

// POST   /monitors
// GET    /monitors
// GET    /monitors/{id}
// DELETE /monitors/{id}

func HandleGetTargets(w http.ResponseWriter, r *http.Request) {
	writeDefaultHeaders(w)
}

func HandleGetTarget(w http.ResponseWriter, r *http.Request) {
	writeDefaultHeaders(w)
}

func HandleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	writeDefaultHeaders(w)
}

func writeDefaultHeaders(w http.ResponseWriter) {
	w.Header().Add("Content-Type", "application/json")
}
