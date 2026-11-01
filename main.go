package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	upstreamURL = env("UPSTREAM_URL", "https://ondemand.st/papi/iptv/playlist.m3u?token=dami2026live")
	upstreamUA  = env("UPSTREAM_UA", "Mozilla/5.0 OnDemandIPTV AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/604.1")
	publicBase  = strings.TrimRight(env("PUBLIC_BASE", ""), "/")
	listen      = ":" + env("PORT", "8080")
	playlistTTL = envDur("PLAYLIST_TTL", 60*time.Second)

	uriAttr = regexp.MustCompile(`URI="([^"]+)"`)
	extOK   = regexp.MustCompile(`^\.[A-Za-z0-9]{1,5}$`)

	client = &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}}

	mu        sync.Mutex
	cachedRaw []byte
	cachedAt  time.Time
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envDur(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return d
}

func newReq(r *http.Request, target string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", upstreamUA)
	req.Header.Set("Accept", "*/*")
	// Range is deliberately not forwarded: segments may carry a junk prefix, so
	// offsets are applied locally after stripping (see streamHandler).
	return req, nil
}

// parseRange handles "bytes=N-" and "bytes=N-M" (end < 0 when open-ended).
func parseRange(h string) (start, end int64, ok bool) {
	if !strings.HasPrefix(h, "bytes=") || strings.Contains(h, ",") {
		return 0, 0, false
	}
	parts := strings.SplitN(strings.TrimPrefix(h, "bytes="), "-", 2)
	if len(parts) != 2 || parts[0] == "" {
		return 0, 0, false
	}
	s, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || s < 0 {
		return 0, 0, false
	}
	e := int64(-1)
	if parts[1] != "" {
		e, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || e < s {
			return 0, 0, false
		}
	}
	return s, e, true
}

func baseOf(r *http.Request) string {
	if publicBase != "" {
		return publicBase
	}
	scheme := "http"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	return scheme + "://" + r.Host
}

var segExts = map[string]bool{
	".ts": true, ".m4s": true, ".mp4": true, ".aac": true, ".mp3": true, ".m4a": true,
	".m4v": true, ".cmfv": true, ".cmfa": true, ".vtt": true, ".webvtt": true, ".key": true,
}

// keepOr keeps a legitimate media extension, otherwise substitutes def.
// This hides fake extensions (.webp, .png, ...) that make ffmpeg-based
// players (mpv, VLC) mis-detect MPEG-TS segments as images.
func keepOr(u *url.URL, def string) string {
	e := strings.ToLower(path.Ext(u.Path))
	if segExts[e] {
		return e
	}
	return def
}

func proxied(base string, ref *url.URL, force string) string {
	enc := base64.RawURLEncoding.EncodeToString([]byte(ref.String()))
	ext := force
	if ext == "" {
		ext = path.Ext(ref.Path)
		if !extOK.MatchString(ext) {
			ext = ""
		}
	}
	return base + "/s/" + enc + ext
}

// rewrite turns every URL in an m3u/m3u8 into a proxied URL, resolving
// relative references against ref.
func rewrite(body []byte, ref *url.URL, base string) []byte {
	text := string(body)
	isMaster := strings.Contains(text, "#EXT-X-STREAM-INF")
	isMedia := !isMaster && strings.Contains(text, "#EXT-X-TARGETDURATION")

	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	// kind: "" (channel list entry), "variant", "segment", "map", "key"
	resolve := func(s, kind string) string {
		u, err := ref.Parse(strings.TrimSpace(s))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return s
		}
		force := ""
		switch kind {
		case "variant":
			force = ".m3u8"
		case "segment":
			force = keepOr(u, ".ts")
		case "map":
			force = keepOr(u, ".mp4")
		case "key":
			force = keepOr(u, ".key")
		}
		return proxied(base, u, force)
	}
	nextVariant := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "#"):
			if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
				nextVariant = true
			}
			if strings.Contains(line, "URI=\"") {
				kind := "segment"
				switch {
				case strings.HasPrefix(line, "#EXT-X-MEDIA"), strings.HasPrefix(line, "#EXT-X-I-FRAME-STREAM-INF"):
					kind = "variant"
				case strings.HasPrefix(line, "#EXT-X-MAP"):
					kind = "map"
				case strings.HasPrefix(line, "#EXT-X-KEY"), strings.HasPrefix(line, "#EXT-X-SESSION-KEY"):
					kind = "key"
				}
				line = uriAttr.ReplaceAllStringFunc(line, func(m string) string {
					sub := uriAttr.FindStringSubmatch(m)
					return `URI="` + resolve(sub[1], kind) + `"`
				})
			}
		default:
			kind := ""
			switch {
			case nextVariant || isMaster:
				kind = "variant"
				nextVariant = false
			case isMedia:
				kind = "segment"
			}
			line = resolve(line, kind)
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// tsOffset returns the offset where MPEG-TS data begins (0 if it already
// starts with a sync byte), or -1 if no TS stream is recognised.
func tsOffset(b []byte) int {
	if len(b) > 0 && b[0] == 0x47 {
		return 0
	}
	for i := 1; i+376 < len(b); i++ {
		if b[i] == 0x47 && b[i+188] == 0x47 && b[i+376] == 0x47 {
			return i
		}
	}
	return -1
}

func fetchPlaylist(r *http.Request) ([]byte, error) {
	mu.Lock()
	defer mu.Unlock()
	if cachedRaw != nil && time.Since(cachedAt) < playlistTTL {
		return cachedRaw, nil
	}
	req, err := newReq(r, upstreamURL)
	if err != nil {
		return cachedRaw, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return cachedRaw, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return cachedRaw, &statusErr{resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return cachedRaw, err
	}
	cachedRaw, cachedAt = b, time.Now()
	log.Printf("playlist refreshed: %d bytes", len(b))
	return b, nil
}

type statusErr struct{ code int }

func (e *statusErr) Error() string { return "upstream status " + strconv.Itoa(e.code) }

func playlistHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := fetchPlaylist(r)
	if raw == nil {
		log.Printf("playlist error: %v", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	if err != nil {
		log.Printf("serving stale playlist: %v", err)
	}
	ref, _ := url.Parse(upstreamURL)
	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(rewrite(raw, ref, baseOf(r)))
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	enc := strings.TrimPrefix(r.URL.Path, "/s/")
	if i := strings.LastIndex(enc, "."); i >= 0 {
		enc = enc[:i]
	}
	dec, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	target := string(dec)
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	req, err := newReq(r, target)
	if err != nil {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("upstream error %s: %v", target, err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	br := bufio.NewReaderSize(resp.Body, 64<<10)
	head, _ := br.Peek(7)
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	isPlaylist := string(head) == "#EXTM3U" || strings.Contains(ct, "mpegurl")

	if isPlaylist && resp.StatusCode == 200 {
		b, err := io.ReadAll(io.LimitReader(br, 8<<20))
		if err != nil {
			http.Error(w, "upstream read error", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(rewrite(b, resp.Request.URL, baseOf(r)))
		return
	}

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// Segments disguised as images: strip any junk prefix before the MPEG-TS
	// data and report an honest content type.
	skip := -1
	if resp.StatusCode == 200 {
		if pk, _ := br.Peek(4096); len(pk) > 0 {
			skip = tsOffset(pk)
		}
	}
	if skip > 0 {
		br.Discard(skip)
		if cl, err := strconv.ParseInt(w.Header().Get("Content-Length"), 10, 64); err == nil && cl >= int64(skip) {
			w.Header().Set("Content-Length", strconv.FormatInt(cl-int64(skip), 10))
		} else {
			w.Header().Del("Content-Length")
		}
		log.Printf("stripped %d-byte prefix from %s", skip, target)
	}
	if skip >= 0 {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	// Apply the client's Range locally (ffmpeg always sends "bytes=0-").
	status := resp.StatusCode
	var body io.Reader = br
	if status == 200 {
		if start, end, ok := parseRange(r.Header.Get("Range")); ok && !(start == 0 && end < 0) {
			total, err := strconv.ParseInt(w.Header().Get("Content-Length"), 10, 64)
			if err == nil && start < total {
				if end < 0 || end >= total {
					end = total - 1
				}
				io.CopyN(io.Discard, br, start)
				body = io.LimitReader(br, end-start+1)
				status = 206
				w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(total, 10))
				w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
			}
		}
	}
	w.WriteHeader(status)
	fl, _ := w.(http.Flusher)
	buf := make([]byte, 64<<10)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
		if rerr != nil {
			return
		}
	}
}

var _ embed.FS

//go:embed index.html
var indexHTML []byte

type channel struct {
	Name  string `json:"name"`
	Group string `json:"group"`
	Logo  string `json:"logo"`
	URL   string `json:"url"`
}

var attrRe = regexp.MustCompile(`([A-Za-z0-9_-]+)="([^"]*)"`)

// extinfName returns the display name after the first comma outside quotes.
func extinfName(s string) string {
	inQ := false
	for i, c := range s {
		if c == '"' {
			inQ = !inQ
		}
		if c == ',' && !inQ {
			return strings.TrimSpace(s[i+1:])
		}
	}
	return ""
}

func parseChannels(raw []byte, ref *url.URL, base string) []channel {
	out := []channel{}
	var cur *channel
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "#EXTINF"):
			c := channel{}
			tvgName := ""
			for _, m := range attrRe.FindAllStringSubmatch(line, -1) {
				switch strings.ToLower(m[1]) {
				case "group-title":
					c.Group = m[2]
				case "tvg-logo":
					c.Logo = m[2]
				case "tvg-name":
					tvgName = m[2]
				}
			}
			c.Name = extinfName(line)
			if c.Name == "" {
				c.Name = tvgName
			}
			cur = &c
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			if cur == nil {
				continue
			}
			u, err := ref.Parse(line)
			if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
				cur.URL = proxied(base, u, "")
				out = append(out, *cur)
			}
			cur = nil
		}
	}
	return out
}

func channelsHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := fetchPlaylist(r)
	if raw == nil {
		log.Printf("channels error: %v", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	ref, _ := url.Parse(upstreamURL)
	base := baseOf(r)
	mu.Lock()
	age := int(time.Since(cachedAt).Seconds())
	mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(map[string]any{
		"playlist":    base + "/playlist.m3u",
		"age_seconds": age,
		"ttl_seconds": int(playlistTTL.Seconds()),
		"channels":    parseChannels(raw, ref, base),
	})
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	mu.Lock()
	cachedAt = time.Time{}
	mu.Unlock()
	channelsHandler(w, r)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/api/channels", channelsHandler)
	mux.HandleFunc("/api/refresh", refreshHandler)
	mux.HandleFunc("/playlist.m3u", playlistHandler)
	mux.HandleFunc("/playlist.m3u8", playlistHandler)
	mux.HandleFunc("/s/", streamHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// no WriteTimeout: live streams are long-lived
	}
	log.Printf("listening on %s (playlist TTL %s)", listen, playlistTTL)
	log.Fatal(srv.ListenAndServe())
}
