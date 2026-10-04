package handlers

import "net/http"

type targetManager interface {
	targetCreator
	targetGetter
	targetLister
	targetDeleter
}

func NewRouter(targets targetManager) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("POST /targets", newCreateTarget(targets))
	mux.Handle("GET /targets", newGetTargets(targets))
	mux.HandleFunc("/targets", methodNotAllowed("GET, HEAD, POST"))

	mux.Handle("GET /targets/{id}", newGetTarget(targets))
	mux.Handle("DELETE /targets/{id}", newDeleteTarget(targets))
	mux.HandleFunc("/targets/{id}", methodNotAllowed("DELETE, GET, HEAD"))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, apiError{
			Code:    codeRouteNotFound,
			Message: "route not found",
		})
	})

	return mux
}

func methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		writeError(w, http.StatusMethodNotAllowed, apiError{
			Code:    codeMethodNotAllowed,
			Message: "method not allowed",
		})
	}
}
