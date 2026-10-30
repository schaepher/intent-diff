package render

import (
	"strings"

	"intent-review/internal/report"
)

// ---- 诊断索引 ----

type warningIndex struct {
	all   []report.Error
	byKey map[string][]report.Error
	chain map[string][]report.Error
}

func newWarningIndex(in []report.Error) *warningIndex {
	idx := &warningIndex{byKey: map[string][]report.Error{}, chain: map[string][]report.Error{}}
	for _, e := range in {
		idx.add(e)
	}
	return idx
}

func (w *warningIndex) add(e report.Error) {
	w.all = append(w.all, e)
	key := e.ChainID + "\x00" + e.IntentID
	w.byKey[key] = append(w.byKey[key], e)
	if e.IntentID == "" {
		w.chain[e.ChainID] = append(w.chain[e.ChainID], e)
	}
}

func (w *warningIndex) decorate(iv *intentView, chainID, intentID string) {
	all := append([]report.Error{}, w.byKey[chainID+"\x00"+intentID]...)
	all = append(all, w.chain[chainID]...)
	for _, e := range all {
		iv.Warnings = append(iv.Warnings, warningView{
			Code: e.Code, Severity: e.Severity, Message: firstNonEmpty(e.Message, e.Hint),
			Repo: e.Repo, File: e.File, Line: e.Line, IsError: !e.Warning(),
		})
	}
	if len(iv.Warnings) > 0 {
		iv.HasWarning = true
		parts := make([]string, 0, len(iv.Warnings))
		for _, wv := range iv.Warnings {
			parts = append(parts, wv.Code+": "+wv.Message)
		}
		iv.WarnTitle = strings.Join(parts, " | ")
	}
}

func (w *warningIndex) flatten() []report.Error { return w.all }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
