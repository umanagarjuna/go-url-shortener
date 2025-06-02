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

	domainReq := &domain.CreateURLRequest{
		URL:      req.Url,
		UserID:   req.UserId,
		Metadata: req.Metadata,
	}

	if *req.ExpiresIn > 0 {
		domainReq.ExpiresIn = req.ExpiresIn
	}

	resp, err := h.service.CreateURL(ctx, domainReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to create URL: %v", err)
	}

	return &pb.URLResponse{
		ShortCode:   resp.ShortCode,
		ShortUrl:    resp.ShortURL,
		OriginalUrl: resp.OriginalURL,
		CreatedAt:   resp.CreatedAt.Unix(),
		ClickCount:  resp.ClickCount,
	}, nil
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
		unixTimestamp := resp.ExpiresAt.Unix() // Get the int64 value
		pbResp.ExpiresAt = &unixTimestamp      // Assign the address of the int64 value
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
