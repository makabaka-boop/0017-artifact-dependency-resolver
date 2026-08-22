package httpapi

import (
	"net/http"
	"strings"

	"artifact-dep-resolver/internal/errcode"
)

// lockfilesCollection 处理 GET/POST /api/v1/lockfiles。
func (s *Server) lockfilesCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, offset := parsePagination(r)
		lfs, err := s.svc.ListLockfiles(limit, offset)
		if err != nil {
			writeErr(w, errcode.New(errcode.Internal, "list failed"))
			return
		}
		writeJSON(w, http.StatusOK, lfs)
	case http.MethodPost:
		var body struct {
			ResolutionID int64  `json:"resolution_id"`
			Name         string `json:"name"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, errcode.New(errcode.BadRequest, "invalid JSON"))
			return
		}
		if body.ResolutionID <= 0 {
			writeErr(w, errcode.New(errcode.BadRequest, "resolution_id required"))
			return
		}
		lf, err := s.svc.CreateLockfile(body.ResolutionID, body.Name)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, lf)
	default:
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
	}
}

// lockfilesItem 处理 GET /api/v1/lockfiles/{name} 及解析校验。
func (s *Server) lockfilesItem(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/lockfiles/")
	if name == "" {
		writeErr(w, errcode.New(errcode.BadRequest, "missing lockfile name"))
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	lf, err := s.svc.GetLockfile(name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lf)
}

// resolveWithLockfile 处理 POST /api/v1/resolve/lockfile。
func (s *Server) resolveWithLockfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	var body struct {
		Lockfile string  `json:"lockfile"`
		Artifact string  `json:"artifact"`
		Version  *string `json:"version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, errcode.New(errcode.BadRequest, "invalid JSON"))
		return
	}
	if body.Lockfile == "" || body.Artifact == "" {
		writeErr(w, errcode.New(errcode.BadRequest, "lockfile and artifact required"))
		return
	}
	out, err := s.svc.ResolveWithLockfile(body.Lockfile, body.Artifact, body.Version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// resolveRerun 处理 POST /api/v1/resolutions/{id}/rerun。
func (s *Server) resolveRerun(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	out, err := s.svc.RerunResolution(id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// dependencyDiff 处理 GET /api/v1/artifacts/{name}/versions/{v1}/diff/{v2}。
func (s *Server) dependencyDiff(w http.ResponseWriter, r *http.Request, name, v1, v2 string) {
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	diff, err := s.svc.DiffDependencies(name, v1, v2)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

// readiness 处理 GET /api/v1/artifacts/{name}/versions/{version}/readiness。
func (s *Server) readiness(w http.ResponseWriter, r *http.Request, name, version string) {
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	report, err := s.svc.CheckReadiness(name, version)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// artifactReadiness 处理无版本的就绪检查（选最高版本）。
func (s *Server) artifactReadiness(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		writeErr(w, errcode.New(errcode.BadRequest, "method not allowed"))
		return
	}
	report, err := s.svc.CheckReadiness(name, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
