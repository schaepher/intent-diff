package gateway

// Audit 记录审计事件。
type Audit struct {
	Events []string
}

// Record 追加一条审计事件。
func (a *Audit) Record(event string) { a.Events = append(a.Events, event) }
