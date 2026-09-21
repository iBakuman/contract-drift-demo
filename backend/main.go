// Command server serves the tiny widget contract three different ways.
//
// All three handlers compile, all three use the generated types, and all three
// look reasonable in review. One of them ships a response the contract forbids.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/iBakuman/contract-drift-demo/backend/api"
)

func main() {
	http.HandleFunc("/widgets", listWidgets)

	log.Println("listening on http://localhost:8099")
	log.Fatal(http.ListenAndServe(":8099", nil))
}

func listWidgets(w http.ResponseWriter, r *http.Request) {
	var body api.WidgetList

	switch r.URL.Query().Get("scenario") {
	case string(api.Ok):
		body = wellBehaved()
	case string(api.NilSlice):
		body = nilSlice()
	case string(api.OmitOptional):
		body = omitOptional()
	default:
		http.Error(w, "scenario must be one of ok, nil-slice, omit-optional", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// wellBehaved builds the slice with make, so an empty result is [] and never
// null. This is the construction discipline that keeps the bug away — as long
// as every author of every handler remembers it.
func wellBehaved() api.WidgetList {
	rows := fetch()

	items := make([]api.Widget, 0, len(rows))
	for _, row := range rows {
		items = append(items, api.Widget{Id: row.id, Label: &row.label})
	}

	return api.WidgetList{GeneratedAt: now(), Items: &items}
}

// nilSlice reaches the same result through a different construction path: the
// slice starts at its zero value and the loop never runs, so it stays nil.
//
// Items is *[]Widget. The pointer is real, so omitempty keeps the key. The
// encoder then asks the slice, and a nil slice encodes as null — a value the
// contract does not allow for this field.
func nilSlice() api.WidgetList {
	byGroup := map[string][]api.Widget{}
	for _, row := range fetch() {
		byGroup[row.group] = append(byGroup[row.group], api.Widget{Id: row.id, Label: &row.label})
	}

	items := byGroup["archived"] // no archived rows today, so this is nil

	return api.WidgetList{GeneratedAt: now(), Items: &items}
}

// omitOptional leaves label out. Every widget here is contract-legal: label is
// optional, and a server may decline to populate it.
func omitOptional() api.WidgetList {
	rows := fetch()

	items := make([]api.Widget, 0, len(rows))
	for _, row := range rows {
		items = append(items, api.Widget{Id: row.id})
	}

	return api.WidgetList{GeneratedAt: now(), Items: &items}
}

type row struct {
	id    string
	label string
	group string
}

func fetch() []row {
	return []row{
		{id: "w-1", label: "left hinge", group: "active"},
		{id: "w-2", label: "right hinge", group: "active"},
	}
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }
