package handler

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/umanagarjuna/go-url-shortener/api/proto/url/v1"
	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
	"github.com/umanagarjuna/go-url-shortener/internal/url/service"
)

type GRPCHandler struct {
	pb.UnimplementedURLServiceServer
	service *service.URLService
}

func NewGRPCHandler(service *service.URLService) *GRPCHandler {
	return &GRPCHandler{
		service: service,
	}
}

func (h *GRPCHandler) CreateURL(ctx context.Context,
	req *pb.CreateURLRequest) (*pb.URLResponse, error) {

	// FIXED: Handle type conversions properly
	domainReq := &domain.CreateURLRequest{
		URL: req.Url,
	}

	// FIXED: Convert *int64 to int64 (with nil check)
	if req.UserId != nil {
		domainReq.UserID = *req.UserId
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "user_id is required")
	}

	// FIXED: Convert map[string]string to map[string]interface{}
	if req.Metadata != nil {
		domainReq.Metadata = make(map[string]interface{})
		for key, value := range req.Metadata {
			domainReq.Metadata[key] = value
		}
	}

	// FIXED: Convert *int64 to *int
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expiresInInt := int(*req.ExpiresIn)
		domainReq.ExpiresIn = &expiresInInt
	}

	resp, err := h.service.CreateURL(ctx, domainReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to create URL: %v", err)
	}

	pbResp := &pb.URLResponse{
		ShortCode:   resp.ShortCode,
		ShortUrl:    resp.ShortURL,
		OriginalUrl: resp.OriginalURL,
		CreatedAt:   resp.CreatedAt.Unix(),
		ClickCount:  resp.ClickCount,
	}

	// Handle ExpiresAt conversion
	if resp.ExpiresAt != nil {
		expiresAtUnix := resp.ExpiresAt.Unix()
		pbResp.ExpiresAt = &expiresAtUnix
	}

	return pbResp, nil
}

func (h *GRPCHandler) GetURL(ctx context.Context,
	req *pb.GetURLRequest) (*pb.URLResponse, error) {

	resp, err := h.service.GetURL(ctx, req.ShortCode)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to get URL: %v", err)
	}

	if resp == nil {
		return nil, status.Errorf(codes.NotFound, "URL not found")
	}

	pbResp := &pb.URLResponse{
		ShortCode:   resp.ShortCode,
		ShortUrl:    resp.ShortURL,
		OriginalUrl: resp.OriginalURL,
		CreatedAt:   resp.CreatedAt.Unix(),
		ClickCount:  resp.ClickCount,
	}

	if resp.ExpiresAt != nil {
		unixTimestamp := resp.ExpiresAt.Unix()
		pbResp.ExpiresAt = &unixTimestamp
	}

	return pbResp, nil
}

func (h *GRPCHandler) ValidateURL(ctx context.Context,
	req *pb.ValidateURLRequest) (*pb.ValidationResponse, error) {

	// Implementation for URL validation
	return &pb.ValidationResponse{
		IsValid: true,
		IsSafe:  true,
	}, nil
}
