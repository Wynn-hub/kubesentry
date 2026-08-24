package service

import "context"

// PlatformInvoker is the narrow compatibility seam for the observability
// domain. Unlike the removed HTTP adapter, it carries no route registration or
// ResponseWriter and can be replaced by repository-backed methods without
// changing generated handlers.
type PlatformInvoker func(context.Context, string, string, []byte) (int, []byte)

type PlatformService struct{ invoke PlatformInvoker }

func NewPlatformService(invoke PlatformInvoker) *PlatformService {
	return &PlatformService{invoke: invoke}
}

func (s *PlatformService) Invoke(ctx context.Context, method, path string, body []byte) (int, []byte) {
	if s == nil || s.invoke == nil {
		return 503, nil
	}
	return s.invoke(ctx, method, path, body)
}
