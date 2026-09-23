package web

import (
	"io"
	"log"
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

func validKind(kind string) bool {
	for _, k := range model.DocumentKinds {
		if k.Key == kind {
			return true
		}
	}
	return false
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	maxBytes := s.cfg.MaxUploadMB << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1<<20)

	// En fazla 4 MB bellekte tutulur, kalani gecici dizine tasar
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		s.setFlash(w, "hata", "Dosya çok büyük veya okunamadı. En fazla "+strconv.FormatInt(s.cfg.MaxUploadMB, 10)+" MB yükleyebilirsiniz.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	if !s.uploadLimit.Allow(s.clientIP(r)) {
		s.renderError(w, r, http.StatusTooManyRequests, "Çok fazla yükleme", "Lütfen bir süre sonra tekrar deneyin.")
		return
	}

	app, period, err := s.currentDraft(r)
	if err != nil {
		http.Redirect(w, r, "/devam", http.StatusSeeOther)
		return
	}
	if app.Status != model.StatusDraft || !period.AcceptingApplications() {
		s.renderError(w, r, http.StatusForbidden, "Başvuru kapalı", "Bu başvuruya belge eklenemez.")
		return
	}

	kind := fstr(r, "kind")
	if !validKind(kind) {
		s.setFlash(w, "hata", "Geçersiz belge türü.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}

	file, header, err := r.FormFile("dosya")
	if err != nil {
		s.setFlash(w, "hata", "Dosya seçilmedi.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}
	defer file.Close()

	if header.Size > maxBytes {
		s.setFlash(w, "hata", "Dosya boyutu en fazla "+strconv.FormatInt(s.cfg.MaxUploadMB, 10)+" MB olabilir.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}

	// Icerige bakarak tur tespiti (uzantiya guvenilmez)
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime := http.DetectContentType(head[:n])
	mime = strings.SplitN(mime, ";", 2)[0]
	ext, ok := allowedUploads[mime]
	if !ok {
		s.setFlash(w, "hata", "Yalnızca PDF, JPG veya PNG dosyası yükleyebilirsiniz.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		s.setFlash(w, "hata", "Dosya okunamadı, tekrar deneyin.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}

	relDir := strconv.FormatUint(uint64(app.ID), 10)
	absDir := filepath.Join(s.cfg.UploadDir(), relDir)
	if err := os.MkdirAll(absDir, 0o750); err != nil {
		log.Printf("yukleme dizini olusturulamadi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Yüklenemedi", "Teknik bir sorun oluştu.")
		return
	}

	stored := filepath.ToSlash(filepath.Join(relDir, kind+"-"+security.RandomToken(8)+ext))
	dst, err := os.OpenFile(filepath.Join(s.cfg.UploadDir(), filepath.FromSlash(stored)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		log.Printf("dosya olusturulamadi: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Yüklenemedi", "Teknik bir sorun oluştu.")
		return
	}
	written, err := io.Copy(dst, io.LimitReader(file, maxBytes))
	closeErr := dst.Close()
	if err != nil || closeErr != nil {
		os.Remove(filepath.Join(s.cfg.UploadDir(), filepath.FromSlash(stored)))
		s.setFlash(w, "hata", "Dosya kaydedilemedi, tekrar deneyin.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}

	// Ayni turden onceki belgeyi diskten temizle
	if old, err := s.st.DocumentMap(ctx, app.ID); err == nil {
		if prev, ok := old[kind]; ok && prev.StoredName != stored {
			_ = os.Remove(filepath.Join(s.cfg.UploadDir(), filepath.FromSlash(prev.StoredName)))
		}
	}

	doc := &model.Document{
		ApplicationID: app.ID,
		Kind:          kind,
		OriginalName:  clip(filepath.Base(header.Filename), 200),
		StoredName:    stored,
		Mime:          mime,
		SizeBytes:     written,
	}
	if err := s.st.SaveDocument(ctx, doc); err != nil {
		log.Printf("belge kaydedilemedi: %v", err)
		s.setFlash(w, "hata", "Belge kaydedilemedi.")
	} else {
		s.setFlash(w, "basarili", model.DocumentKindLabel(kind)+" yüklendi.")
	}
	http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
}

func (s *Server) handleUploadDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusForbidden, "Doğrulama hatası", "Sayfayı yenileyip tekrar deneyin.")
		return
	}
	app, period, err := s.currentDraft(r)
	if err != nil {
		http.Redirect(w, r, "/devam", http.StatusSeeOther)
		return
	}
	if app.Status != model.StatusDraft || !period.AcceptingApplications() {
		s.renderError(w, r, http.StatusForbidden, "İşlem yapılamaz", "Bu başvuru üzerinde değişiklik yapılamaz.")
		return
	}

	id, _ := strconv.ParseUint(fstr(r, "id"), 10, 32)
	doc, err := s.st.DocumentByID(ctx, uint32(id))
	if err != nil || doc.ApplicationID != app.ID {
		s.setFlash(w, "hata", "Belge bulunamadı.")
		http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
		return
	}
	if _, err := s.st.DeleteDocument(ctx, doc.ID); err == nil {
		_ = os.Remove(filepath.Join(s.cfg.UploadDir(), filepath.FromSlash(doc.StoredName)))
		s.setFlash(w, "bilgi", model.DocumentKindLabel(doc.Kind)+" silindi.")
	}
	http.Redirect(w, r, "/basvuru/adim/6", http.StatusSeeOther)
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

	u := userFrom(r)
	actor := "bilinmiyor"
	if u != nil {
		actor = u.Username
	}
	s.st.Audit(r.Context(), actor, "belge_goruntulendi", "document", strconv.Itoa(int(doc.ID)), doc.Kind, s.clientIP(r))

	w.Header().Set("Content-Type", doc.Mime)
	w.Header().Set("Content-Disposition", "inline; filename=\""+sanitizeFilename(doc.OriginalName)+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, doc.OriginalName, doc.UploadedAt, f)
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	if name == "" {
		return "belge"
	}
	return clip(name, 100)
}
