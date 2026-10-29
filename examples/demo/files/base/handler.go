package gateway

import "net/http"

// Handler 是网关入口。
func Handler(w http.ResponseWriter, r *http.Request) {
	resp := callDownstream(r)
	w.Write(resp)
}

func callDownstream(r *http.Request) []byte {
	return nil
}
