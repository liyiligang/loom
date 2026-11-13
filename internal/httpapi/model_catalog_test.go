package httpapi_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/liyiligang/loom/internal/model"
)

func TestModelCatalogIsManualDurableAndIndependentOfScopeSelections(t *testing.T) {
	f := setup(t)
	var catalog model.ModelCatalog
	f.call("GET", "/settings/models", nil, 200, &catalog)
	if catalog.Revision != 0 || len(catalog.Models) != 0 {
		t.Fatal("model settings were prepopulated")
	}
	id := requirementForChat(t, f)
	f.call("PATCH", "/requirements/"+id+"/models", map[string]any{"models": map[string]string{"codex": "manual-model"}}, 200, nil)
	var feature struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &feature)
	f.submit(feature.ID, "catalog-running", "Run the scoped fixture")
	before := f.snapshot(feature.ID)
	values := []model.ConfiguredModel{{Provider: "codex", Name: "  我的开发模型  ", ID: "  manual-model  "}, {Provider: "claude", Name: "Review model", ID: "manual-model"}}
	f.call("PUT", "/settings/models", map[string]any{"revision": 0, "models": values}, 200, &catalog)
	if catalog.Revision != 1 || catalog.Models[0].Name != "我的开发模型" || catalog.Models[0].ID != "manual-model" {
		t.Fatal("catalog normalization failed")
	}
	var bootstrap struct {
		ModelCatalog model.ModelCatalog
		Requirements []model.Requirement
		Sessions     []model.Session
	}
	f.call("GET", "/bootstrap", nil, 200, &bootstrap)
	if !reflect.DeepEqual(bootstrap.ModelCatalog, catalog) || len(bootstrap.Sessions) != 1 {
		t.Fatal("settings were not published or created a native session")
	}
	f.call("PUT", "/settings/models", map[string]any{"revision": 0, "models": []model.ConfiguredModel{}}, 409, nil)
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(feature.ID).Session.State == "running" })
	var restored model.ModelCatalog
	f.call("GET", "/settings/models", nil, 200, &restored)
	if !reflect.DeepEqual(catalog, restored) {
		t.Fatal("model catalog did not survive restart")
	}
	f.call("PUT", "/settings/models", map[string]any{"revision": catalog.Revision, "models": []model.ConfiguredModel{}}, 200, &catalog)
	current := f.snapshot(feature.ID)
	if current.Session.ActiveTurn != before.Session.ActiveTurn || *current.Turns[0].Model != "manual-model" {
		t.Fatal("catalog replacement changed an admitted turn")
	}
	finishModelTurn(t, f, feature.ID)
	f.submit(feature.ID, "catalog-after-removal", "Continue with the saved requirement model")
	assertNativeModel(t, f, "turn/start", "manual-model")
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	for _, request := range f.native.requests {
		if request.Method == "model/list" {
			t.Fatal("manual catalog requested provider discovery")
		}
	}
}

func TestModelCatalogRejectsInvalidAndDuplicateEntriesAtomically(t *testing.T) {
	f := setup(t)
	valid := model.ConfiguredModel{Provider: "codex", Name: "Development", ID: "model-id"}
	cases := []any{
		map[string]any{}, map[string]any{"models": nil},
		map[string]any{"models": []model.ConfiguredModel{{Provider: "other", Name: "Model", ID: "id"}}},
		map[string]any{"models": []model.ConfiguredModel{{Provider: "codex", Name: " ", ID: "id"}}},
		map[string]any{"models": []model.ConfiguredModel{{Provider: "codex", Name: "Model", ID: " "}}},
		map[string]any{"models": []model.ConfiguredModel{{Provider: "codex", Name: "Model", ID: "x y"}}},
		map[string]any{"models": []model.ConfiguredModel{{Provider: "codex", Name: "Model", ID: "x\x00y"}}},
		map[string]any{"models": []model.ConfiguredModel{{Provider: "codex", Name: "Model", ID: strings.Repeat("x", 121)}}},
		map[string]any{"models": []model.ConfiguredModel{valid, valid}},
		map[string]any{"models": make([]model.ConfiguredModel, 201)},
	}
	for _, input := range cases {
		f.call("PUT", "/settings/models", input, 400, nil)
	}
	var catalog model.ModelCatalog
	f.call("GET", "/settings/models", nil, 200, &catalog)
	if catalog.Revision != 0 || len(catalog.Models) != 0 {
		t.Fatal("invalid input changed model settings")
	}
	f.call("PUT", "/settings/models", map[string]any{"revision": 0, "models": []model.ConfiguredModel{valid}}, 200, &catalog)
	f.call("PUT", "/settings/models", map[string]any{"revision": catalog.Revision, "models": []model.ConfiguredModel{{Provider: "codex", Name: "Renamed", ID: "new-id"}}}, 200, &catalog)
	if catalog.Revision != 2 || catalog.Models[0].Name != "Renamed" || catalog.Models[0].ID != "new-id" {
		t.Fatal("model list editing failed")
	}
}
