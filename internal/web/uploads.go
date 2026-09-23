package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lafed/burs/internal/model"
	"github.com/lafed/burs/internal/security"
)

var allowedUploads = map[string]string{
	"application/pdf": ".pdf",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
}

// storeDocument basvuru formuyla birlikte gelen belgeyi diske ve veritabanina yazar.
// Donen hata mesaji dogrudan ogrenciye gosterilecek bicimdedir.
func (s *Server) storeDocument(ctx context.Context, appID uint32, kind string, file multipart.File, header *multipart.FileHeader) error {
	maxBytes := s.cfg.MaxUploadMB << 20
	if header.Size > maxBytes {
		return fmt.Errorf("dosya en fazla %d MB olabilir", s.cfg.MaxUploadMB)
	}

	// Turu icerikten tespit et; uzantiya guvenilmez
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime := strings.SplitN(http.DetectContentType(head[:n]), ";", 2)[0]
	ext, ok := allowedUploads[mime]
	if !ok {
		return errors.New("yalnızca PDF, JPG veya PNG dosyası yükleyebilirsiniz")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return errors.New("dosya okunamadı, tekrar deneyin")
	}

	relDir := strconv.FormatUint(uint64(appID), 10)
	if err := os.MkdirAll(filepath.Join(s.cfg.UploadDir(), relDir), 0o750); err != nil {
		log.Printf("yukleme dizini olusturulamadi: %v", err)
		return errors.New("dosya kaydedilemedi, tekrar deneyin")
	}

	stored := filepath.ToSlash(filepath.Join(relDir, kind+"-"+security.RandomToken(8)+ext))
	full := filepath.Join(s.cfg.UploadDir(), filepath.FromSlash(stored))

	dst, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		log.Printf("dosya olusturulamadi: %v", err)
		return errors.New("dosya kaydedilemedi, tekrar deneyin")
	}
	written, copyErr := io.Copy(dst, io.LimitReader(file, maxBytes))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(full)
		return errors.New("dosya kaydedilemedi, tekrar deneyin")
	}

	doc := &model.Document{
		ApplicationID: appID,
		Kind:          kind,
		OriginalName:  clip(filepath.Base(header.Filename), 200),
		StoredName:    stored,
		Mime:          mime,
		SizeBytes:     written,
	}
	if err := s.st.SaveDocument(ctx, doc); err != nil {
		_ = os.Remove(full)
		log.Printf("belge kaydedilemedi: %v", err)
		return errors.New("belge kaydedilemedi, tekrar deneyin")
	}
	return nil
}

// handleDocDownload belgeleri yalnizca oturum acmis yetkililere sunar.
func (s *Server) handleDocDownload(w http.ResponseWriter, r *http.Request) {
	doc, err := s.st.DocumentByID(r.Context(), fid(r, "id"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Belge bulunamadı", "Bu belge kayıtlarda yok.")
		return
	}
	// Depolanan yol her zaman uygulama tarafindan uretilir; yine de dizin disina cikisi engelle
	full := filepath.Join(s.cfg.UploadDir(), filepath.FromSlash(doc.StoredName))
	if !strings.HasPrefix(full, filepath.Clean(s.cfg.UploadDir())+string(os.PathSeparator)) {
		s.renderError(w, r, http.StatusBadRequest, "Geçersiz istek", "Belge yolu geçersiz.")
		return
	}
	f, err := os.Open(full)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Belge bulunamadı", "Dosya diskte bulunamadı.")
		return
	}
	defer f.Close()

	actor := "bilinmiyor"
	if u := userFrom(r); u != nil {
		actor = u.Username
	}
	s.st.Audit(r.Context(), actor, "belge_goruntulendi", "document",
		strconv.Itoa(int(doc.ID)), doc.Kind, s.clientIP(r))

	w.Header().Set("Content-Type", doc.Mime)
	w.Header().Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(doc.OriginalName)+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, doc.OriginalName, doc.UploadedAt, f)
}

func sanitizeFilename(name string) string {
	name = strings.NewReplacer("\"", "", "\n", "", "\r", "").Replace(name)
	if name == "" {
		return "belge"
	}
	return clip(name, 100)
}
