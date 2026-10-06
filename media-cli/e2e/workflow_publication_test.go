package e2e

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Registers only a unique definition copied from an authorized source. No Job,
// provider call, Worker, source edit or credential creation occurs here.
func publicationFixture(t *testing.T) (string, string, string) {
	t.Helper()
	address, token, source := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_PUBLICATION_SOURCE_ID")
	if address == "" || token == "" || source == "" {
		t.Skip("gateway, API Token and authorized publication source required")
	}
	status, data := doJSONBearer(t, address, "GET", "/api/v1/workflows/"+source, token, nil)
	var w struct {
		Scope  string           `json:"scope"`
		Output string           `json:"output_step_key"`
		Steps  []map[string]any `json:"steps"`
	}
	if status != 200 || json.Unmarshal(data, &w) != nil || w.Scope == "" || len(w.Steps) == 0 {
		t.Fatal("authorized source read failed")
	}
	status, data = doJSONBearer(t, address, "POST", "/api/v1/workflows?scope="+url.QueryEscape(w.Scope), token, map[string]any{"name": "publication-gate-" + time.Now().UTC().Format("20060102T150405.000000000"), "output_step_key": w.Output, "steps": w.Steps})
	var created struct {
		ID string `json:"id"`
	}
	if status != 200 || json.Unmarshal(data, &created) != nil || created.ID == "" {
		t.Fatal("publication fixture creation failed")
	}
	t.Log("publication fixture workflow_id=" + created.ID)
	t.Cleanup(func() {
		status, _ := doJSONBearer(t, address, "POST", "/api/v1/workflows/"+created.ID+"/archive", token, map[string]any{})
		if status != 200 {
			t.Error("publication fixture archive failed")
		}
	})
	return address, token, created.ID
}

type publicationReceipt struct {
	ID                string `json:"id"`
	Version           int    `json:"version"`
	Status            int    `json:"status"`
	PublicationID     string `json:"publication_id"`
	PublicationStatus string `json:"publication_status"`
}

func TestWorkflowPublicationGateway(t *testing.T) {
	address, token, id := publicationFixture(t)
	var wg sync.WaitGroup
	receipts := make(chan publicationReceipt, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, data := doJSONBearer(t, address, "POST", "/api/v1/workflows/"+id+"/publish", token, map[string]any{})
			var receipt publicationReceipt
			if (status != 200 && status != 202) || json.Unmarshal(data, &receipt) != nil || receipt.PublicationID == "" || receipt.Version != 1 {
				t.Error("concurrent publication not accepted")
				return
			}
			if status == 202 && receipt.PublicationStatus != "pending" {
				t.Error("202 without pending receipt")
			}
			receipts <- receipt
		}()
	}
	wg.Wait()
	close(receipts)
	count, receiptID := 0, ""
	for receipt := range receipts {
		count++
		if receiptID != "" && receipt.PublicationID != receiptID {
			t.Fatal("duplicate version has different receipt")
		}
		receiptID = receipt.PublicationID
	}
	if count != 8 {
		t.Fatal("not all concurrent publication calls acknowledged")
	}
	for deadline := time.Now().Add(30 * time.Second); ; {
		status, data := doJSONBearer(t, address, "GET", "/api/v1/workflows/"+id, token, nil)
		var receipt publicationReceipt
		if status != 200 || json.Unmarshal(data, &receipt) != nil || receipt.PublicationID != receiptID {
			t.Fatal("publication query lost receipt")
		}
		if receipt.Status == 2 && receipt.PublicationStatus == "registered" {
			break
		}
		if receipt.PublicationStatus == "failed" || time.Now().After(deadline) {
			t.Fatal("publication did not converge; do not resubmit or replace its version")
		}
		time.Sleep(100 * time.Millisecond)
	}
	status, data := doJSONBearer(t, address, "POST", "/api/v1/workflows/"+id+"/publish", token, map[string]any{})
	var replay publicationReceipt
	if status != 200 || json.Unmarshal(data, &replay) != nil || replay.PublicationID != receiptID || replay.PublicationStatus != "registered" {
		t.Fatal("registered replay failed")
	}
	status, _ = doJSONBearer(t, address, "POST", "/api/v1/workflows/"+id+"/archive", token, map[string]any{})
	if status != 200 {
		t.Fatal("archive failed")
	}
	status, _ = doJSONBearer(t, address, "POST", "/api/v1/workflows/"+id+"/publish", token, map[string]any{})
	if status != 409 {
		t.Fatal("archived source republished")
	}
}

func TestWorkflowPublicationCLI(t *testing.T) {
	binary := os.Getenv("MEDIA_E2E_CLI_BINARY")
	if !filepath.IsAbs(binary) {
		t.Skip("independent CLI binary required")
	}
	address, token, id := publicationFixture(t)
	call := func(operation string) struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Receipt  string `json:"publication_id"`
		Delivery string `json:"publication_status"`
	} {
		t.Helper()
		cmd := exec.Command(binary, "media_workflow", "workflow", operation, "--id", id)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "APP_ENV=local", "MEDIA_WORKFLOW_ADDR="+address, "MEDIA_MATERIALS_ADDR="+address, "MEDIA_API_TOKEN="+token)
		output, err := cmd.Output()
		var receipt struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Receipt  string `json:"publication_id"`
			Delivery string `json:"publication_status"`
		}
		if err != nil || json.Unmarshal(output, &receipt) != nil || receipt.ID != id || receipt.Receipt == "" {
			t.Fatal("independent publication CLI failed; output suppressed")
		}
		return receipt
	}
	accepted := call("publish") // Single publication command, never retry on uncertainty.
	for deadline := time.Now().Add(30 * time.Second); ; {
		receipt := call("get")
		if receipt.Receipt != accepted.Receipt {
			t.Fatal("CLI receipt identity changed")
		}
		if receipt.Status == "WF_PUBLISHED" && receipt.Delivery == "registered" {
			break
		}
		if receipt.Delivery == "failed" || time.Now().After(deadline) {
			t.Fatal("CLI publication did not converge; query existing workflow")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
