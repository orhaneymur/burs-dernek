// LAFED Burs Başvuru ve Değerlendirme Sistemi
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // saat dilimi verisi binary icine gomulur (scratch imaji)

	"github.com/lafed/burs/internal/config"
	"github.com/lafed/burs/internal/security"
	"github.com/lafed/burs/internal/store"
	"github.com/lafed/burs/internal/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[burs] ")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("yapilandirma hatasi: %v", err)
	}

	keys, err := security.NewKeyring(cfg.Secret)
	if err != nil {
		log.Fatalf("anahtar hatasi: %v", err)
	}

	st, err := store.Open(cfg.DSN, keys)
	if err != nil {
		log.Fatalf("veritabani hatasi: %v", err)
	}
	defer st.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := st.Migrate(ctx); err != nil {
		cancel()
		log.Fatalf("gocme hatasi: %v", err)
	}
	if err := st.EnsureBootstrapAdmin(ctx, cfg.BootstrapUser, cfg.BootstrapPass); err != nil {
		cancel()
		log.Fatalf("yonetici hesabi olusturulamadi: %v", err)
	}
	cancel()

	srv, err := web.New(cfg, st, keys)
	if err != nil {
		log.Fatalf("sunucu hazirlanamadi: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute, // belge yuklemeleri icin genis
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	// Suresi dolmus oturumlari periyodik temizle
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				st.PurgeExpiredSessions(c)
				cancel()
			case <-stop:
				return
			}
		}
	}()

	go func() {
		log.Printf("dinleniyor: %s  (veri dizini: %s)", cfg.Addr, cfg.DataDir)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("sunucu hatasi: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Print("kapatiliyor...")

	close(stop)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("kapatma hatasi: %v", err)
	}
	log.Print("kapandi")
}
