package hotdigest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func completion(content string) string {
	payload := map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"content": content}}},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func newModelServer(t *testing.T, status int, content string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
			return
		}
		_, _ = w.Write([]byte(completion(content)))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newTestEnricher(t *testing.T, srv *httptest.Server) *Enricher {
	t.Helper()
	// Retries are part of the client, but a test must not wait for them.
	return &Enricher{APIURL: srv.URL, APIKey: "test-key", Model: "test-model", Client: srv.Client(), RetryDelay: time.Millisecond}
}

func aiCandidate(id, title string) Item {
	return Item{ID: id, Title: title, Group: GroupAI, Kind: KindNews, Lang: "en", Source: "Wire"}
}

func TestCurateKeepsModelOrderAndWritesEnglish(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusOK,
		`[{"id":"b","title":"A smaller model for coding","summary":"It runs locally."},`+
			`{"id":"a","title":"An inference cost breakdown","summary":"Cheaper per token."}]`)
	e := newTestEnricher(t, srv)
	pool := []Item{aiCandidate("a", "Model release"), aiCandidate("b", "Another model")}

	got := e.Curate(context.Background(), GroupAI, pool, 2)
	if len(got) != 2 {
		t.Fatalf("items = %d, want 2", len(got))
	}
	if got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("model order not preserved: %s, %s", got[0].ID, got[1].ID)
	}
	if got[0].Title != "A smaller model for coding" || got[0].TitleOrig != "Another model" {
		t.Errorf("title = %q / original %q", got[0].Title, got[0].TitleOrig)
	}
	if !got[0].Rewritten {
		t.Error("Rewritten flag not set")
	}
	if got[1].Summary != "Cheaper per token." {
		t.Errorf("summary = %q", got[1].Summary)
	}
}

func TestCurateRejectsChineseForEnglishStory(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusOK, `[{"id":"a","title":"一个更小的编程模型","summary":"可以本地运行。"}]`)
	e := newTestEnricher(t, srv)
	got := e.Curate(context.Background(), GroupAI, []Item{aiCandidate("a", "A smaller model")}, 1)
	if len(got) != 1 {
		t.Fatalf("items = %d", len(got))
	}
	if got[0].Title != "A smaller model" || got[0].Summary != "" {
		t.Fatalf("a Chinese rewrite leaked into an English story: %+v", got[0])
	}
}

func TestCurateKeepsChineseForChineseStory(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusOK, `[{"id":"a","title":"国产大模型开源新权重","summary":"参数更小，可本地运行。"}]`)
	e := newTestEnricher(t, srv)
	item := Item{ID: "a", Title: "量子位：新模型发布", Group: GroupAI, Kind: KindNews, Lang: "zh", Source: "量子位"}
	got := e.Curate(context.Background(), GroupAI, []Item{item}, 1)
	if got[0].Title != "国产大模型开源新权重" {
		t.Fatalf("title = %q", got[0].Title)
	}
	if got[0].Summary != "参数更小，可本地运行。" {
		t.Errorf("summary = %q", got[0].Summary)
	}
}

func TestCurateRejectsEnglishForChineseStory(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusOK, `[{"id":"a","title":"A new open-weight model","summary":"Smaller weights."}]`)
	e := newTestEnricher(t, srv)
	item := Item{ID: "a", Title: "国产大模型开源新权重", Group: GroupAI, Lang: "zh", Source: "量子位"}
	got := e.Curate(context.Background(), GroupAI, []Item{item}, 1)
	if got[0].Title != "国产大模型开源新权重" {
		t.Fatalf("an English rewrite replaced a Chinese title: %q", got[0].Title)
	}
}

func TestCurateFallsBackOnModelFailure(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusInternalServerError, "")
	e := newTestEnricher(t, srv)
	if got := e.Curate(context.Background(), GroupAI, []Item{aiCandidate("a", "Keep me")}, 1); got != nil {
		t.Fatalf("a failing model must leave the rule selection in charge, got %+v", got)
	}
}

func TestCurateDropsUnknownIDs(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusOK, `[{"id":"ghost","title":"Not in the pool"},{"id":"a","title":"Real one","summary":"Fine."}]`)
	e := newTestEnricher(t, srv)
	got := e.Curate(context.Background(), GroupAI, []Item{aiCandidate("a", "Real")}, 5)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("unknown ids were not dropped: %+v", got)
	}
}

func TestCurateReturnsNilWhenNothingMatches(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusOK, `[{"id":"ghost","title":"Not in the pool"}]`)
	e := newTestEnricher(t, srv)
	if got := e.Curate(context.Background(), GroupAI, []Item{aiCandidate("a", "Real")}, 5); got != nil {
		t.Fatalf("expected nil so the caller keeps its own selection, got %+v", got)
	}
}

func TestCurateTrimsToRequestedSize(t *testing.T) {
	rows := `[{"id":"a","title":"A","summary":""},{"id":"b","title":"B","summary":""},{"id":"c","title":"C","summary":""}]`
	srv, _ := newModelServer(t, http.StatusOK, rows)
	e := newTestEnricher(t, srv)
	pool := []Item{aiCandidate("a", "a"), aiCandidate("b", "b"), aiCandidate("c", "c")}
	if got := e.Curate(context.Background(), GroupAI, pool, 2); len(got) != 2 {
		t.Fatalf("items = %d, want 2", len(got))
	}
}

func TestCurateWithoutKeyIsNoOp(t *testing.T) {
	if e := NewEnricher("  "); e != nil {
		t.Fatal("an empty API key must disable curation")
	}
	var e *Enricher
	if got := e.Curate(context.Background(), GroupAI, []Item{aiCandidate("a", "T")}, 1); got != nil {
		t.Fatal("nil enricher returned items")
	}
}

func TestTranslateConvertsForeignTitles(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusOK,
		`[{"index":0,"title":"Hegel and the social construction of reality","summary":"An argument about social ontology."},`+
			`{"index":1,"title":"The ethics of Stoic emotions","summary":"A study of Barlaam."}]`)
	e := newTestEnricher(t, srv)
	items := []Item{
		{ID: "a", Title: "Hegel et la construction sociale de la réalité", Lang: "fr", Summary: "Un argument."},
		{ID: "b", Title: "L’éthique et la théorie des émotions stoïciennes", Lang: "fr"},
	}
	got := e.Translate(context.Background(), items)
	if got[0].Title != "Hegel and the social construction of reality" {
		t.Errorf("title = %q", got[0].Title)
	}
	if !got[0].Translated || !got[0].Rewritten {
		t.Error("translation not flagged")
	}
	if got[0].TitleOrig != "Hegel et la construction sociale de la réalité" {
		t.Errorf("original = %q", got[0].TitleOrig)
	}
	if got[0].Summary != "An argument about social ontology." {
		t.Errorf("summary = %q", got[0].Summary)
	}
	if got[1].Title != "The ethics of Stoic emotions" {
		t.Errorf("second title = %q", got[1].Title)
	}
}

func TestTranslateKeepsForeignTextWhenModelFails(t *testing.T) {
	srv, _ := newModelServer(t, http.StatusInternalServerError, "")
	e := newTestEnricher(t, srv)
	items := []Item{{ID: "a", Title: "Kant-Studien: Zwischen Baumgarten und Kant", Lang: "de"}}
	got := e.Translate(context.Background(), items)
	if got[0].Title != "Kant-Studien: Zwischen Baumgarten und Kant" {
		t.Fatalf("a failed translation must leave the original: %q", got[0].Title)
	}
	if got[0].Translated {
		t.Error("nothing was translated, but the flag is set")
	}
}

func TestTranslateRejectsForeignOutput(t *testing.T) {
	// A model that echoes the French title must not be accepted as English.
	srv, _ := newModelServer(t, http.StatusOK, `[{"index":0,"title":"Hegel et la construction sociale de la réalité","summary":""}]`)
	e := newTestEnricher(t, srv)
	items := []Item{{ID: "a", Title: "Hegel et la construction sociale de la réalité", Lang: "fr"}}
	got := e.Translate(context.Background(), items)
	if got[0].Translated {
		t.Fatalf("a French echo was accepted as a translation: %q", got[0].Title)
	}
}

func TestTranslateSkipsChineseAndEnglish(t *testing.T) {
	srv, hits := newModelServer(t, http.StatusOK, `[{"index":0,"title":"x","summary":""}]`)
	e := newTestEnricher(t, srv)
	items := []Item{
		{ID: "zh", Title: "国产大模型开源新权重", Lang: "zh"},
		{ID: "en", Title: "An Argument for Minimalism about Action", Lang: "en"},
	}
	if got := e.Translate(context.Background(), items); got[0].Title != items[0].Title || got[1].Title != items[1].Title {
		t.Error("Chinese and English items must be left alone")
	}
	if *hits != 0 {
		t.Errorf("model was called %d times for items that need no translation", *hits)
	}
}

func TestLooksEnglish(t *testing.T) {
	english := []string{
		"An Argument for Minimalism about Action",
		"Probability for Trivalent Conditionals",
		"Kant-Studien",
	}
	for _, s := range english {
		if !looksEnglish(s) {
			t.Errorf("%q should read as English", s)
		}
	}
	foreign := []string{
		"Hegel et la construction sociale de la réalité",
		"Zwischen Baumgarten und Kant",
		"El salón de Gascón",
		"Poesía mexicana actual",
	}
	for _, s := range foreign {
		if looksEnglish(s) {
			t.Errorf("%q should not read as English", s)
		}
	}
}

func TestParseCurationToleratesFence(t *testing.T) {
	text := "结果：\n```json\n[{\"id\":\"a\",\"title\":\"T1\",\"summary\":\"S1\"},{\"id\":\"b\",\"title\":\"T2\"}]\n```\n以上。"
	got, err := parseCuration(text)
	if err != nil {
		t.Fatalf("parseCuration: %v", err)
	}
	if len(got) != 2 || got[0].Title != "T1" || got[1].ID != "b" {
		t.Fatalf("parsed = %+v", got)
	}
	if _, err := parseCuration("no json here"); err == nil {
		t.Error("expected an error without a JSON array")
	}
}

func TestTargetIsChinese(t *testing.T) {
	if !targetIsChinese(Item{Lang: "zh"}) {
		t.Error("zh sources must stay Chinese")
	}
	if targetIsChinese(Item{Lang: "en"}) {
		t.Error("en sources must stay English")
	}
	if !targetIsChinese(Item{Title: "国产大模型开源新权重"}) {
		t.Error("CJK titles default to Chinese")
	}
	if targetIsChinese(Item{Title: "Open-weight model"}) {
		t.Error("Latin titles default to English")
	}
}
