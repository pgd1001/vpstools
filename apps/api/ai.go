package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/ai"
	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/redact"
)

const (
	defaultAIMaxPromptBytes = 16 * 1024
	defaultAIMaxResponse    = 64 * 1024
	maxAIEvidenceItems      = 10
	maxAIEvidenceBytes      = 64 * 1024
)

var apiAIProvider ai.Provider

type aiAnalysisRequest struct {
	Question    string        `json:"question"`
	ExecutionID string        `json:"execution_id,omitempty"`
	Evidence    []ai.Evidence `json:"evidence,omitempty"`
}

func handleAIAnalyze(w http.ResponseWriter, r *http.Request) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if apiAIProvider == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AI provider is not configured", "next": "Set AI_PROVIDER=openai-compatible, AI_ENDPOINT, and AI_MODEL on the API."})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var request aiAnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid AI analysis request"})
		return
	}
	request.Question = strings.TrimSpace(request.Question)
	if request.Question == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "question is required"})
		return
	}
	if len([]byte(request.Question)) > aiPromptLimit() {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "question exceeds the configured AI prompt limit"})
		return
	}
	if len(request.Evidence) > maxAIEvidenceItems {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many evidence items"})
		return
	}
	if request.ExecutionID != "" {
		evidence, err := executionAIEvidence(r.Context(), dbFrom(r), actor.OrganisationID, request.ExecutionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "execution evidence is not available"})
			return
		}
		request.Evidence = append(request.Evidence, evidence...)
	}
	if evidenceBytes(request.Evidence) > maxAIEvidenceBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "evidence exceeds the configured limit"})
		return
	}

	analysisID := "ai_" + shortID()
	providerRequest := ai.Request{
		Model:        "",
		SystemPrompt: "You are a read-only VPS operations analyst. Use only the supplied evidence. Do not recommend or perform changes. State uncertainty clearly and identify missing evidence.",
		UserPrompt:   request.Question,
		Evidence:     request.Evidence,
		Metadata:     map[string]string{"analysis_id": analysisID, "organisation_id": actor.OrganisationID},
	}
	started := time.Now()
	response, err := apiAIProvider.Complete(r.Context(), providerRequest)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		persistAIRequest(r.Context(), dbFrom(r), analysisID, actor, request, "failed", "", response.Model, response.RequestID, duration, err.Error())
		writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "ai.analysis.failed", "ai_analysis", analysisID, "failed", map[string]any{"execution_id": request.ExecutionID, "evidence_count": len(request.Evidence), "duration_ms": duration})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "AI provider request failed"})
		return
	}
	if int64(len([]byte(response.Text))) > aiResponseLimit() {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "AI provider response exceeded the configured limit"})
		return
	}
	if err := persistAIRequest(r.Context(), dbFrom(r), analysisID, actor, request, "succeeded", response.Text, response.Model, response.RequestID, duration, ""); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist AI analysis"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "ai.analysis.completed", "ai_analysis", analysisID, "succeeded", map[string]any{"execution_id": request.ExecutionID, "evidence_count": len(request.Evidence), "model": response.Model, "provider_request_id": response.RequestID, "duration_ms": duration})
	writeJSON(w, http.StatusOK, map[string]any{"analysis_id": analysisID, "text": response.Text, "model": response.Model, "request_id": response.RequestID, "usage": response.Usage, "evidence_count": len(request.Evidence), "read_only": true})
}

func aiPromptLimit() int {
	if apiBackends.AIMaxPromptBytes > 0 {
		return apiBackends.AIMaxPromptBytes
	}
	return defaultAIMaxPromptBytes
}

func aiResponseLimit() int64 {
	if apiBackends.AIMaxResponseBytes > 0 {
		return apiBackends.AIMaxResponseBytes
	}
	return defaultAIMaxResponse
}

func evidenceBytes(items []ai.Evidence) int {
	total := 0
	for _, item := range items {
		total += len([]byte(item.Title)) + len([]byte(item.Content)) + len([]byte(item.SourceURI))
	}
	return total
}

func executionAIEvidence(ctx context.Context, db *sql.DB, orgID, executionID string) ([]ai.Evidence, error) {
	rows, err := apiQuery(ctx, db, `SELECT et.id, s.name, et.stdout, et.stderr FROM execution_targets et JOIN executions e ON e.id = et.execution_id JOIN servers s ON s.id = et.server_id WHERE e.id = ? AND e.organisation_id = ? ORDER BY et.id`, executionID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ai.Evidence
	for rows.Next() {
		var id, name, stdout, stderr string
		if err := rows.Scan(&id, &name, &stdout, &stderr); err != nil {
			return nil, err
		}
		content := redact.Stdout(strings.TrimSpace(stdout + "\n" + stderr))
		result = append(result, ai.Evidence{ID: id, Kind: "execution_output", Title: "Execution output for " + name, Content: content, SourceURI: "execution:" + executionID + "/target:" + id})
	}
	if err := rows.Err(); err != nil || len(result) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("no execution evidence")
	}
	return result, nil
}

func persistAIRequest(ctx context.Context, db *sql.DB, id string, actor *authz.Actor, request aiAnalysisRequest, status, response, model, providerRequestID string, duration int64, errorSummary string) error {
	requestJSON, _ := json.Marshal(aiAnalysisRequest{Question: redact.Stdout(request.Question), ExecutionID: request.ExecutionID})
	response = redact.Stdout(response)
	tx, err := beginAPITx(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = apiExec(ctx, tx, `INSERT INTO ai_requests (id, organisation_id, actor_user_id, status, request_json, response_text, model, provider_request_id, duration_ms, error_summary) VALUES (?,?,?,?,`+metadataRuntime().JSONParameter()+`,?,?,?,?,?)`, id, actor.OrganisationID, actor.UserID, status, string(requestJSON), response, model, providerRequestID, duration, errorSummary)
	if err != nil {
		return fmt.Errorf("insert AI request: %w", err)
	}
	for i, item := range request.Evidence {
		_, err = apiExec(ctx, tx, `INSERT INTO ai_evidence (id, request_id, organisation_id, ordinal, kind, title, content, source_uri) VALUES (?,?,?,?,?,?,?,?)`, id+"_ev_"+fmt.Sprint(i+1), id, actor.OrganisationID, i, item.Kind, redact.Stdout(item.Title), redact.Stdout(item.Content), item.SourceURI)
		if err != nil {
			return fmt.Errorf("insert AI evidence: %w", err)
		}
	}
	return tx.Commit()
}
