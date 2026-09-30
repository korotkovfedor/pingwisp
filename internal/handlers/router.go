package handlers

import "net/http"

type TargetManager interface {
	TargetCreator
	TargetGetter
	TargetLister
	TargetDeleter
}

func NewRouter(targets TargetManager) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("POST /targets", NewCreateTarget(targets))
	mux.Handle("GET /targets", NewGetTargets(targets))
	mux.HandleFunc("/targets", methodNotAllowed("GET, HEAD, POST"))

	mux.Handle("GET /targets/{id}", NewGetTarget(targets))
	mux.Handle("DELETE /targets/{id}", NewDeleteTarget(targets))
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
