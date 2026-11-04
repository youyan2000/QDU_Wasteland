package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"wasteland/internal/tools/history"
)

type visitRequest struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Path  string `json:"path"`
	Title string `json:"title"`
}

func validateVisitRequest(body *visitRequest) error {
	parsed, err := url.ParseRequestURI(body.Path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || len(body.Path) > 512 {
		return fmt.Errorf("页面路径无效")
	}
	if body.Kind != "article" && body.Kind != "post" && body.Kind != "course" && body.Kind != "file" {
		return fmt.Errorf("页面类型无效")
	}
	if strings.TrimSpace(body.ID) == "" || len(body.ID) > 128 {
		return fmt.Errorf("页面编号无效")
	}
	expected := map[string]string{"article": "/article.html", "post": "/post.html", "course": "/course.html", "file": "/viewer.html"}[body.Kind]
	if parsed.Path != expected {
		return fmt.Errorf("页面路径与类型不匹配")
	}
	query := parsed.Query()
	if body.Kind == "course" {
		if query.Get("code") != body.ID {
			return fmt.Errorf("课程编号与页面路径不匹配")
		}
	} else {
		pathID, parseErr := strconv.ParseInt(query.Get("id"), 10, 64)
		bodyID, bodyErr := strconv.ParseInt(body.ID, 10, 64)
		if parseErr != nil || bodyErr != nil || pathID <= 0 || pathID != bodyID {
			return fmt.Errorf("内容编号与页面路径不匹配")
		}
	}
	body.Title = strings.TrimSpace(body.Title)
	if len(body.Title) > 240 {
		body.Title = body.Title[:240]
	}
	if body.Title == "" {
		body.Title = body.Path
	}
	return nil
}

func handleRecordVisit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			apiErr(w, http.StatusMethodNotAllowed, "只支持 POST")
			return
		}
		uid := currentUserID(r)
		if uid <= 0 {
			apiJSON(w, http.StatusOK, map[string]bool{"recorded": false})
			return
		}
		if u, err := authStore.GetByID(uid); err != nil || u == nil || u.Banned != 0 {
			apiErr(w, http.StatusUnauthorized, "登录状态无效")
			return
		}
		var body visitRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			apiErr(w, http.StatusBadRequest, "请求格式错误")
			return
		}
		if err := validateVisitRequest(&body); err != nil {
			apiErr(w, http.StatusBadRequest, err.Error())
			return
		}
		var exists int
		var title string
		var err error
		switch body.Kind {
		case "article":
			err = authStore.db.QueryRow(`SELECT title FROM articles WHERE id=? AND status IN ('', '正常')`, body.ID).Scan(&title)
		case "post":
			err = authStore.db.QueryRow(`SELECT title FROM forum_posts WHERE id=? AND status IN ('', '正常')`, body.ID).Scan(&title)
		case "course":
			if db != nil {
				if c, ok := db.byCode[body.ID]; ok && c != nil {
					exists = 1
					title = c.Name
				}
			}
		case "file":
			err = authStore.db.QueryRow(`SELECT COUNT(*) FROM files WHERE id=? AND status IN ('', '正常')`, body.ID).Scan(&exists)
		}
		if body.Kind == "article" || body.Kind == "post" {
			if err != nil {
				apiErr(w, http.StatusNotFound, "内容不存在")
				return
			}
			body.Title = title
		} else if (body.Kind == "file" || body.Kind == "course") && (err != nil || exists != 1) {
			apiErr(w, http.StatusNotFound, "内容不存在")
			return
		} else if body.Kind == "course" {
			body.Title = title
		}
		recorded, err := history.RecordSiteVisit(authStore.db, uid, body.Path, body.Title, time.Now())
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "足迹写入失败")
			return
		}
		apiJSON(w, http.StatusOK, map[string]bool{"recorded": recorded})
	}
}
