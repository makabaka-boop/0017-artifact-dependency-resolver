package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"artifact-dep-resolver/internal/errcode"
	"artifact-dep-resolver/internal/service"
)

// Server 持有业务服务并路由 HTTP 请求。
type Server struct {
	svc *service.Service
}

// New 构造 Server。
func New(svc *service.Service) *Server {
	return &Server{svc: svc}
}

// Handler 返回完整的 HTTP handler（含路由）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/v1/artifacts", s.artifactsCollection)
	mux.HandleFunc("/api/v1/artifacts/", s.artifactsItem)
	mux.HandleFunc("/api/v1/resolve", s.resolve)
	mux.HandleFunc("/api/v1/resolve/lockfile", s.resolveWithLockfile)
	mux.HandleFunc("/api/v1/resolutions", s.resolutionsCollection)
	mux.HandleFunc("/api/v1/resolutions/", s.resolutionsItem)
	mux.HandleFunc("/api/v1/changes", s.changesCollection)
	mux.HandleFunc("/api/v1/changes/", s.changesItem)
	mux.HandleFunc("/api/v1/lockfiles", s.lockfilesCollection)
	mux.HandleFunc("/api/v1/lockfiles/", s.lockfilesItem)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "down", "db": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "db": "up"})
}

func (s *Server) artifactsCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, offset := parsePagination(r)
		arts, err := s.svc.ListArtifacts(limit, offset)
		if err != nil {
			writeErr(w, errcode.New(errcode.Internal, "list failed"))
			return
		}
		writeJSON(w, http.StatusOK, arts)
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, errcode.New(errcode.BadRequest, "invalid JSON"))
			return
		}
		a, err := s.svc.CreateArtifact(body.Name)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, a)
	default:
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
	}
}

func (s *Server) artifactsItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/artifacts/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		writeErr(w, errcode.New(errcode.BadRequest, "missing artifact name"))
		return
	}
	name := segments[0]

	switch {
	case len(segments) == 1:
		if r.Method == http.MethodGet {
			a, err := s.svc.GetArtifact(name)
			if err != nil {
				writeErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, a)
			return
		}
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
	case len(segments) == 2 && segments[1] == "versions":
		s.versionsCollection(w, r, name)
	case len(segments) == 2 && segments[1] == "dependencies":
		s.dependencyGraph(w, r, name, "")
	case len(segments) == 2 && segments[1] == "readiness":
		s.artifactReadiness(w, r, name)
	case len(segments) == 3 && segments[1] == "versions" && segments[2] == "dependencies":
		writeErr(w, errcode.New(errcode.BadRequest, "missing version"))
	case len(segments) == 4 && segments[1] == "versions" && segments[3] == "dependencies":
		version := segments[2]
		if r.Method == http.MethodGet {
			s.listDependencies(w, r, name, version)
		} else if r.Method == http.MethodPut {
			s.putDependencies(w, r, name, version)
		} else {
			writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		}
	case len(segments) == 4 && segments[1] == "versions" && segments[3] == "readiness":
		s.readiness(w, r, name, segments[2])
	case len(segments) == 5 && segments[1] == "versions" && segments[3] == "diff":
		s.dependencyDiff(w, r, name, segments[2], segments[4])
	case len(segments) == 3 && segments[1] == "versions":
		version := segments[2]
		if r.Method == http.MethodGet {
			s.getVersionDetail(w, r, name, version)
			return
		}
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
	default:
		writeErr(w, errcode.New(errcode.NotFound, "not found"))
	}
}

func (s *Server) versionsCollection(w http.ResponseWriter, r *http.Request, name string) {
	switch r.Method {
	case http.MethodGet:
		vers, err := s.svc.ListVersions(name)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, vers)
	case http.MethodPost:
		var body struct {
			Version string `json:"version"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, errcode.New(errcode.BadRequest, "invalid JSON"))
			return
		}
		v, err := s.svc.CreateVersion(name, body.Version)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	default:
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
	}
}

func (s *Server) getVersionDetail(w http.ResponseWriter, r *http.Request, name, version string) {
	v, err := s.svc.GetVersion(name, version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) listDependencies(w http.ResponseWriter, r *http.Request, name, version string) {
	deps := s.svc.Dependencies(name, version)
	writeJSON(w, http.StatusOK, deps)
}

func (s *Server) putDependencies(w http.ResponseWriter, r *http.Request, name, version string) {
	var body struct {
		Dependencies []service.DependencyInput `json:"dependencies"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, errcode.New(errcode.BadRequest, "invalid JSON"))
		return
	}
	if err := s.svc.ReplaceDependencies(name, version, body.Dependencies); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) dependencyGraph(w http.ResponseWriter, r *http.Request, name, version string) {
	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	g, err := s.svc.DependencyGraph(name, version, depth)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	var body struct {
		Artifact string  `json:"artifact"`
		Version  *string `json:"version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, errcode.New(errcode.BadRequest, "invalid JSON"))
		return
	}
	if body.Artifact == "" {
		writeErr(w, errcode.New(errcode.BadRequest, "artifact required"))
		return
	}
	out, err := s.svc.Resolve(body.Artifact, body.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) resolutionsCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	limit, offset := parsePagination(r)
	status := r.URL.Query().Get("status")
	rs, err := s.svc.ListResolutions(limit, offset, status)
	if err != nil {
		writeErr(w, errcode.New(errcode.Internal, "list failed"))
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) resolutionsItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/resolutions/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		writeErr(w, errcode.New(errcode.BadRequest, "missing id"))
		return
	}
	id, err := strconv.ParseInt(segments[0], 10, 64)
	if err != nil {
		writeErr(w, errcode.New(errcode.BadRequest, "invalid id"))
		return
	}
	if len(segments) == 2 && segments[1] == "rollback" {
		if r.Method != http.MethodPost {
			writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
			return
		}
		nr, err := s.svc.Rollback(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nr)
		return
	}
	if len(segments) == 2 && segments[1] == "rerun" {
		s.resolveRerun(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	res, items, err := s.svc.GetResolution(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"resolution": res, "items": items})
}

func (s *Server) changesCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	limit, offset := parsePagination(r)
	et := r.URL.Query().Get("entity_type")
	eid, _ := strconv.ParseInt(r.URL.Query().Get("entity_id"), 10, 64)
	rs, err := s.svc.ListChanges(limit, offset, et, eid)
	if err != nil {
		writeErr(w, errcode.New(errcode.Internal, "list failed"))
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) changesItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/changes/")
	id, err := strconv.ParseInt(strings.Trim(path, "/"), 10, 64)
	if err != nil {
		writeErr(w, errcode.New(errcode.BadRequest, "invalid id"))
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	c, err := s.svc.GetChange(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func parsePagination(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func decodeJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeErr(w http.ResponseWriter, e error) {
	var apiErr *errcode.APIError
	if ae, ok := e.(*errcode.APIError); ok {
		apiErr = ae
	} else {
		apiErr = errcode.New(errcode.Internal, "internal error")
	}
	writeJSON(w, apiErr.Code.HTTPStatus(), map[string]interface{}{"error": apiErr})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
