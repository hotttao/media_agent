package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"gorm.io/gorm"
	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/dal"
	userrepo "media_agent/media_auth/biz/dal/repo/auth"
	platformrepo "media_agent/media_auth/biz/dal/repo/platform_token"
	platformhandler "media_agent/media_auth/biz/handler/platform_token"
	platformrouter "media_agent/media_auth/biz/router/platform_token"
	platformservice "media_agent/media_auth/biz/service/platform_token"
	"media_agent/media_auth/biz/shared/platformcipher"
	"media_agent/media_auth/biz/shared/platformmtls"
	"media_agent/media_auth/biz/workflow"
)

var errPlatformRuntime = errors.New("platform token runtime initialization failed")

type platformRuntime struct {
	server   *server.Hertz
	listener net.Listener
	sink     *workflow.PlatformAuditFileSink
	delivery *workflow.PlatformAuditDelivery
	service  *platformservice.Service
	cancel   context.CancelFunc
	done     chan struct{}
	stopping atomic.Bool
}

// Initialize before public routing. Disabled means no files, listeners or
// globals are touched. Enabling requires an already prepared custody cutover.
func newPlatformRuntime(db *gorm.DB, cfg *configpb.PlatformTokenConfig) (*platformRuntime, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, nil
	}
	if db == nil || cfg.GetAddress() == "" {
		return nil, errPlatformRuntime
	}
	ring, err := platformcipher.LoadFile(cfg.GetKeyFile(), cfg.GetKeyVersion())
	if err != nil {
		return nil, errPlatformRuntime
	}
	v, tlsConfig, err := platformmtls.LoadFiles(cfg.GetServerCertFile(), cfg.GetServerKeyFile(), cfg.GetClientCaFile(), map[string]string{
		"spiffe://media-agent/services/media_agent_gateway":           "media_agent_gateway",
		"spiffe://media-agent/services/media_control_schedule_worker": "media_control_schedule_worker",
	})
	if err != nil {
		return nil, errPlatformRuntime
	}
	sink, err := workflow.OpenPlatformAuditFileSink(cfg.GetAuditFile())
	if err != nil {
		return nil, errPlatformRuntime
	}
	repo, tx := platformrepo.NewRepo(db), dal.NewTxRunner(db)
	delivery, err := workflow.NewPlatformAuditDelivery(tx, repo, sink, workflow.PlatformAuditOptions{})
	if err != nil {
		sink.Close()
		return nil, errPlatformRuntime
	}
	listener, err := net.Listen("tcp", cfg.GetAddress())
	if err != nil {
		sink.Close()
		return nil, errPlatformRuntime
	}
	svc := platformservice.NewService(repo, repo, userrepo.NewPlatformUserReader(db), tx, ring, v, platformservice.Options{})
	h := newPlatformServer(listener, tlsConfig, svc, v)
	return &platformRuntime{server: h, listener: listener, sink: sink, delivery: delivery, service: svc}, nil
}

func newPlatformServer(listener net.Listener, tlsConfig *tls.Config, svc platformhandler.Service, v *platformmtls.Verifier) *server.Hertz {
	h := server.New(server.WithListener(listener), server.WithTLS(tlsConfig), server.WithReadTimeout(5*time.Second), server.WithWriteTimeout(5*time.Second), server.WithIdleTimeout(30*time.Second), server.WithMaxRequestBodySize(4096), server.WithDisablePrintRoute(true))
	platformhandler.PlatformSvc, platformhandler.PlatformVerifier = svc, v
	platformrouter.CallerVerifier = v
	platformrouter.Register(h)
	return h
}

func (r *platformRuntime) start() {
	if r == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.done = cancel, make(chan struct{})
	go func() { defer close(r.done); r.delivery.Run(ctx) }()
}

func (r *platformRuntime) close() {
	if r == nil {
		return
	}
	r.stopping.Store(true)
	if r.cancel != nil {
		r.cancel()
		<-r.done
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.server.Shutdown(ctx)
	_ = r.listener.Close()
	_ = r.sink.Close()
}
