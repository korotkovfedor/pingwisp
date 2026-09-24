
func HandleAddTarget(w http.ResponseWriter, r *http.Request) {
	writeDefaultHeaders(w)
}

type AddTargetRequest struct {
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

func (r *AddTargetRequest) Validate() error {
	if r.URL == "" {
		return errors.New("url is required")
	}

	if r.IntervalSeconds <= 0 {
		return errors.New("interval_seconds must be greater than 0")
	}

	return nil
}

