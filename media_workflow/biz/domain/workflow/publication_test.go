package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func validPublication() Publication {
	p := Publication{ID: "pub", WorkflowID: "workflow", WorkflowVersion: 1, CompilationID: "compiled", RequestedBy: "owner", DefinitionHash: strings.Repeat("a", 64),
		DefinitionJSON: json.RawMessage(`{"name":"workflow","version":1,"tasks":[]}`), TaskDefinitionsJSON: json.RawMessage(`[{"name":"render","retryCount":2}]`)}
	p.PayloadDigest, _ = p.Digest()
	return p
}

func TestPublicationFrozenIdentityAndJSONDigest(t *testing.T) {
	p := validPublication()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	replay := p
	replay.DefinitionJSON = json.RawMessage(`{ "tasks":[], "version":1, "name":"workflow" }`)
	replay.TaskDefinitionsJSON = json.RawMessage(`[{"retryCount":2,"name":"render"}]`)
	replay.Status, replay.ClaimToken = "registered", "another-lease"
	if replay.Validate() != nil {
		t.Fatal("formatting or delivery state changed intent identity")
	}
	for _, change := range []func(*Publication){
		func(p *Publication) { p.RequestedBy = "other" },
		func(p *Publication) { p.WorkflowVersion++ },
		func(p *Publication) { p.DefinitionHash = strings.Repeat("b", 64) },
		func(p *Publication) { p.DefinitionJSON = json.RawMessage(`{"name":"changed","version":1}`) },
		func(p *Publication) { p.TaskDefinitionsJSON = json.RawMessage(`[{"name":"render","retryCount":9}]`) },
	} {
		changed := p
		change(&changed)
		if changed.Validate() == nil {
			t.Fatal("changed frozen payload accepted with original digest")
		}
	}
}

func TestPublicationRejectsInvalidCommands(t *testing.T) {
	for _, change := range []func(*Publication){
		func(p *Publication) { p.ID = " pub " },
		func(p *Publication) { p.WorkflowVersion = 0 },
		func(p *Publication) { p.DefinitionJSON = json.RawMessage(`[]`) },
		func(p *Publication) { p.DefinitionJSON = json.RawMessage(`{"name":"a","version":0}`) },
		func(p *Publication) { p.DefinitionJSON = json.RawMessage(`{"name":"a","version":1.1}`) },
		func(p *Publication) { p.DefinitionJSON = json.RawMessage(`{"name":"a","version":1} {}`) },
		func(p *Publication) { p.TaskDefinitionsJSON = json.RawMessage(`[]`) },
		func(p *Publication) { p.TaskDefinitionsJSON = json.RawMessage(`[{"name":"a"},{"name":"a"}]`) },
		func(p *Publication) { p.TaskDefinitionsJSON = json.RawMessage(`[{"name":""}]`) },
		func(p *Publication) { p.DefinitionJSON = json.RawMessage(strings.Repeat(" ", (4<<20)+1)) },
	} {
		p := validPublication()
		change(&p)
		p.PayloadDigest, _ = p.Digest()
		if p.Validate() == nil {
			t.Fatal("invalid command accepted")
		}
	}
}
