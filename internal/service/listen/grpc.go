// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	messagesv1 "github.com/canonical/authorization-service/api/v1"
	"github.com/canonical/authorization-service/internal/model/permissions"
	"github.com/canonical/authorization-service/internal/service/authz"
)

const authorizationMetadataKey = "authorization"

// GRPCIngestionServer implements messagesv1.PermissionIngestionServiceServer,
// authenticating callers via Ory Hydra Bearer JWTs and delegating envelope
// ingestion to the EnvelopeIngestor.
type GRPCIngestionServer struct {
	messagesv1.UnimplementedPermissionIngestionServiceServer

	verifier authz.OidcVerifier
	ingestor EnvelopeIngestor
	logger   *slog.Logger
}

// NewGRPCIngestionServer constructs a GRPCIngestionServer.
func NewGRPCIngestionServer(
	verifier authz.OidcVerifier,
	ingestor EnvelopeIngestor,
	logger *slog.Logger,
) *GRPCIngestionServer {
	return &GRPCIngestionServer{
		verifier: verifier,
		ingestor: ingestor,
		logger:   logger,
	}
}

// Register registers the PermissionIngestionService with the gRPC server.
func (s *GRPCIngestionServer) Register(grpcServer *grpc.Server) {
	messagesv1.RegisterPermissionIngestionServiceServer(grpcServer, s)
}

// IngestPermissionUpdate authenticates the incoming Bearer JWT against the
// configured Hydra JWKS endpoint and persists the permission-update envelope.
func (s *GRPCIngestionServer) IngestPermissionUpdate(
	ctx context.Context,
	req *messagesv1.PermissionUpdateEnvelope,
) (*messagesv1.IngestPermissionUpdateResponse, error) {
	token, err := extractBearerTokenFromContext(ctx)
	if err != nil {
		s.logger.DebugContext(ctx, "Missing or malformed authorization metadata on gRPC ingestion request", "error", err)
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	if s.verifier == nil {
		s.logger.ErrorContext(ctx, "OIDC verifier is not configured for gRPC ingestion")
		return nil, status.Error(codes.Unauthenticated, "authentication verifier not configured")
	}

	if _, err := s.verifier.Verify(ctx, token); err != nil {
		s.logger.DebugContext(ctx, "Failed to verify Bearer JWT on gRPC ingestion request", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid bearer token")
	}

	if err := s.ingestor.IngestEnvelope(ctx, req); err != nil {
		if permissions.IsPermanent(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		s.logger.ErrorContext(ctx, "Transient failure persisting gRPC permission update", "error", err)
		return nil, status.Error(codes.Internal, "failed to persist permission update")
	}

	return &messagesv1.IngestPermissionUpdateResponse{}, nil
}

func extractBearerTokenFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing request metadata")
	}

	values := md.Get(authorizationMetadataKey)
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization metadata")
	}

	authHeader := strings.TrimSpace(values[0])
	if len(authHeader) < 7 || !strings.EqualFold(authHeader[:7], "Bearer ") {
		return "", status.Error(codes.Unauthenticated, "authorization metadata must use Bearer scheme")
	}

	token := strings.TrimSpace(authHeader[7:])
	if token == "" {
		return "", status.Error(codes.Unauthenticated, "empty bearer token")
	}

	return token, nil
}
