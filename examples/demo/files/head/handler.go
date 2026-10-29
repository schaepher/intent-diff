package gateway

import "net/http"

// Handler 是网关入口。
func Handler(w http.ResponseWriter, r *http.Request) {
	logf("request start")
	limiter := newTokenBucket(cfg)
	if !limiter.Allow() {
		logf("rate limited")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	resp := callDownstream(r)
	logf("request done")
	w.Write(resp)
}

func callDownstream(r *http.Request) []byte {
	return nil
}
