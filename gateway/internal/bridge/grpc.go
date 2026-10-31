// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	gosov1 "github.com/mqglobal/goso/gateway/internal/bridge/gosov1"
	"github.com/mqglobal/goso/gateway/internal/llm"
)

const (
	procGenerate = "/goso.v1.GOSOBridgeService/GenerateReply"
	procStream   = "/goso.v1.GOSOBridgeService/StreamReply"
	procHealth   = "/goso.v1.GOSOBridgeService/CheckHealth"
)

func (s *Service) generateReply(ctx context.Context, req *connect.Request[gosov1.GenerateReplyRequest]) (*connect.Response[gosov1.GenerateReplyResponse], error) {
	msg := req.Msg
	if msg == nil {
		msg = &gosov1.GenerateReplyRequest{}
	}
	if err := s.verify(req.Header(), []byte(msg.GetPrompt())); err != nil {
		return nil, connectErr(err)
	}
	in, err := replyFromProto(req.Header(), msg)
	if err != nil {
		return nil, connectErr(err)
	}
	// Proto3 cannot omit temperature; the Omni client always sets it.
	in.TemperatureSet = true
	out, err := s.Reply(ctx, in, nil)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&gosov1.GenerateReplyResponse{
		ReplyText:        out.ReplyText,
		PromptTokens:     out.PromptTokens,
		CompletionTokens: out.CompletionTokens,
		LatencyMs:        out.LatencyMS,
		StopReason:       out.StopReason,
		DelayMs:          out.DelayMS,
	}), nil
}

func (s *Service) streamReply(ctx context.Context, req *connect.Request[gosov1.GenerateReplyRequest], stream *connect.ServerStream[gosov1.StreamReplyChunk]) error {
	msg := req.Msg
	if msg == nil {
		msg = &gosov1.GenerateReplyRequest{}
	}
	if err := s.verify(req.Header(), []byte(msg.GetPrompt())); err != nil {
		return connectErr(err)
	}
	in, err := replyFromProto(req.Header(), msg)
	if err != nil {
		return connectErr(err)
	}
	in.TemperatureSet = true
	sent := false
	out, err := s.Reply(ctx, in, func(delta string) {
		if delta == "" {
			return
		}
		sent = true
		_ = stream.Send(&gosov1.StreamReplyChunk{Delta: delta})
	})
	if err != nil {
		return connectErr(err)
	}
	if !sent && out.ReplyText != "" {
		if err := stream.Send(&gosov1.StreamReplyChunk{Delta: out.ReplyText}); err != nil {
			return err
		}
	}
	return stream.Send(&gosov1.StreamReplyChunk{IsFinished: true, StopReason: out.StopReason})
}

func (s *Service) checkHealth(_ context.Context, _ *connect.Request[gosov1.HealthRequest]) (*connect.Response[gosov1.HealthResponse], error) {
	return connect.NewResponse(&gosov1.HealthResponse{
		Healthy: true,
		Version: s.version(),
	}), nil
}

func replyFromProto(h http.Header, msg *gosov1.GenerateReplyRequest) (ReplyInput, error) {
	headerTenant := strings.TrimSpace(h.Get(headerTenant))
	bodyTenant := strings.TrimSpace(msg.GetTenantId())
	if headerTenant == "" {
		headerTenant = bodyTenant
	}
	if headerTenant == "" {
		return ReplyInput{}, unauthorized("missing x-goclaw-tenant")
	}
	if bodyTenant != "" && headerTenant != bodyTenant {
		return ReplyInput{}, unauthorized("tenant mismatch")
	}
	hist := make([]llm.Message, 0, len(msg.GetHistory()))
	for _, m := range msg.GetHistory() {
		if m == nil {
			continue
		}
		hist = append(hist, llm.Message{Role: m.GetRole(), Content: m.GetContent()})
	}
	agentID := meta(msg.GetMetadata(), "agent_id", "agentId")
	if agentID == "" {
		agentID = strings.TrimSpace(h.Get(headerAgent))
	}
	return ReplyInput{
		TenantID:         headerTenant,
		AgentID:          agentID,
		SessionID:        msg.GetSessionId(),
		ChannelAccountID: msg.GetChannelAccountId(),
		ContactID:        msg.GetContactId(),
		Prompt:           msg.GetPrompt(),
		Model:            msg.GetModel(),
		SystemPrompt:     msg.GetSystemPrompt(),
		Temperature:      msg.GetTemperature(),
		History:          hist,
		Metadata:         msg.GetMetadata(),
	}, nil
}

func connectErr(err error) error {
	if err == nil {
		return nil
	}
	var ce *connect.Error
	if errors.As(err, &ce) {
		return err
	}
	code := connect.CodeInternal
	msg := "internal error"
	var be *Error
	if errors.As(err, &be) && be != nil {
		msg = be.Msg
		if be.Code != "" {
			msg = be.Code + ": " + be.Msg
		}
		switch be.Code {
		case "insufficient_funds":
			code = connect.CodeFailedPrecondition
		case "rate_limited":
			code = connect.CodeResourceExhausted
		case "upstream_unauthorized":
			code = connect.CodeUnauthenticated
		case "upstream_forbidden":
			code = connect.CodePermissionDenied
		case "upstream_not_found":
			code = connect.CodeNotFound
		case "empty_reply", "upstream_unavailable":
			code = connect.CodeUnavailable
		case "upstream_rejected":
			code = connect.CodeInvalidArgument
		default:
			switch be.Status {
			case http.StatusUnauthorized:
				code = connect.CodeUnauthenticated
			case http.StatusBadRequest:
				code = connect.CodeInvalidArgument
			case http.StatusNotFound:
				code = connect.CodeNotFound
			case http.StatusConflict:
				code = connect.CodeFailedPrecondition
			case http.StatusRequestEntityTooLarge:
				code = connect.CodeResourceExhausted
			case http.StatusServiceUnavailable, http.StatusBadGateway:
				code = connect.CodeUnavailable
			}
		}
	}
	return connect.NewError(code, errors.New(msg))
}
