// bluebook is a JSON-first client for Blue Book users and their AI agents.
package main

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultAPIURL = "http://localhost:8080/api/v1"

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return nil
	}
	c := newClient()
	switch args[0] {
	case "api":
		return runAPI(c, args[1:])
	case "post":
		return runPost(c, args[1:])
	case "comment":
		return runComment(c, args[1:])
	case "media":
		return runMedia(c, args[1:])
	case "search":
		return runSearch(c, args[1:])
	case "me":
		return get(c, "/me/profile")
	case "feed":
		return get(c, "/feed/recommended"+query(args[1:]))
	case "like", "unlike", "collect", "uncollect", "follow", "unfollow":
		return runAction(c, args)
	default:
		return fmt.Errorf("未知命令 %q\n\n%s", args[0], usage)
	}
}

func newClient() *client {
	baseURL := strings.TrimRight(os.Getenv("BLUEBOOK_API_URL"), "/")
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	token := os.Getenv("BLUEBOOK_API_KEY")
	if token == "" {
		token = os.Getenv("BLUEBOOK_ACCESS_TOKEN")
	}
	return &client{baseURL: baseURL, token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *client) request(method, path string, body []byte, contentType string) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if len(payload) == 0 {
			return nil, fmt.Errorf("请求失败: %s", resp.Status)
		}
		return nil, fmt.Errorf("请求失败: %s: %s", resp.Status, payload)
	}
	return payload, nil
}

func writeResponse(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	if !jsontext.Value(payload).IsValid() {
		return errors.New("服务返回了无效 JSON")
	}
	_, err := os.Stdout.Write(append(payload, '\n'))
	return err
}

func call(c *client, method, path string, value any) error {
	var body []byte
	if value != nil {
		var buf bytes.Buffer
		if err := json.MarshalWrite(&buf, value); err != nil {
			return err
		}
		body = buf.Bytes()
	}
	payload, err := c.request(method, path, body, "application/json")
	if err != nil {
		return err
	}
	return writeResponse(payload)
}

func get(c *client, path string) error { return call(c, http.MethodGet, path, nil) }

func runAPI(c *client, args []string) error {
	if len(args) < 2 {
		return errors.New("用法: bluebook api <get|post|put|patch|delete> <path> [--data JSON]")
	}
	method := strings.ToUpper(args[0])
	if method != "GET" && method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return errors.New("HTTP 方法必须是 get、post、put、patch 或 delete")
	}
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	data := fs.String("data", "", "JSON request body")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	var body []byte
	if *data != "" {
		if !jsontext.Value(*data).IsValid() {
			return errors.New("--data 必须是合法 JSON")
		}
		body = []byte(*data)
	}
	payload, err := c.request(method, args[1], body, "application/json")
	if err != nil {
		return err
	}
	return writeResponse(payload)
}

func runPost(c *client, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: bluebook post <create|list|get|update|delete>")
	}
	switch args[0] {
	case "list":
		return get(c, "/posts"+query(args[1:]))
	case "get":
		if len(args) != 2 {
			return errors.New("用法: bluebook post get <post-id>")
		}
		return get(c, "/posts/"+args[1])
	case "delete":
		if len(args) != 2 {
			return errors.New("用法: bluebook post delete <post-id>")
		}
		return call(c, http.MethodDelete, "/posts/"+args[1], nil)
	case "create", "update":
		return runPostWrite(c, args)
	default:
		return errors.New("用法: bluebook post <create|list|get|update|delete>")
	}
}

type stringList []string

func (v *stringList) String() string         { return strings.Join(*v, ",") }
func (v *stringList) Set(value string) error { *v = append(*v, value); return nil }

func runPostWrite(c *client, args []string) error {
	update := args[0] == "update"
	start := 1
	postID := ""
	if update {
		if len(args) < 2 {
			return errors.New("用法: bluebook post update <post-id> --title TITLE --content CONTENT")
		}
		postID, start = args[1], 2
	}
	fs := flag.NewFlagSet("post "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	title := fs.String("title", "", "post title")
	content := fs.String("content", "", "post content")
	var tags, images, videos stringList
	fs.Var(&tags, "tag", "tag (repeatable)")
	fs.Var(&images, "image", "uploaded image object key (repeatable)")
	fs.Var(&videos, "video", "uploaded video object key (repeatable)")
	if err := fs.Parse(args[start:]); err != nil {
		return err
	}
	if *title == "" || *content == "" {
		return errors.New("--title 和 --content 必填")
	}
	media := make([]map[string]any, 0, len(images)+len(videos))
	for _, key := range images {
		media = append(media, map[string]any{"media_key": key, "media_type": "image", "sort_order": len(media)})
	}
	for _, key := range videos {
		media = append(media, map[string]any{"media_key": key, "media_type": "video", "sort_order": len(media)})
	}
	body := map[string]any{"title": *title, "content": *content, "tags": tags, "media": media}
	if update {
		return call(c, http.MethodPatch, "/posts/"+postID, body)
	}
	return call(c, http.MethodPost, "/posts", body)
}

func runComment(c *client, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: bluebook comment <create|list|delete>")
	}
	switch args[0] {
	case "list":
		if len(args) < 2 {
			return errors.New("用法: bluebook comment list <post-id> [--page N]")
		}
		return get(c, "/posts/"+args[1]+"/comments"+query(args[2:]))
	case "delete":
		if len(args) != 2 {
			return errors.New("用法: bluebook comment delete <comment-id>")
		}
		return call(c, http.MethodDelete, "/comments/"+args[1], nil)
	case "create":
		fs := flag.NewFlagSet("comment create", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		postID := fs.String("post", "", "post ID")
		parentID := fs.String("parent", "", "parent comment ID")
		content := fs.String("content", "", "content")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *postID == "" || *content == "" {
			return errors.New("--post 和 --content 必填")
		}
		body := map[string]any{"post_id": *postID, "content": *content}
		if *parentID != "" {
			body["parent_id"] = *parentID
		}
		return call(c, http.MethodPost, "/comments", body)
	default:
		return errors.New("用法: bluebook comment <create|list|delete>")
	}
}

func runAction(c *client, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("用法: bluebook %s <id>", args[0])
	}
	method, path := http.MethodPut, ""
	switch args[0] {
	case "like":
		path = "/posts/" + args[1] + "/like"
	case "unlike":
		method, path = http.MethodDelete, "/posts/"+args[1]+"/like"
	case "collect":
		path = "/posts/" + args[1] + "/collection"
	case "uncollect":
		method, path = http.MethodDelete, "/posts/"+args[1]+"/collection"
	case "follow":
		path = "/users/" + args[1] + "/follow"
	case "unfollow":
		method, path = http.MethodDelete, "/users/"+args[1]+"/follow"
	}
	return call(c, method, path, nil)
}

func runSearch(c *client, args []string) error {
	if len(args) == 0 {
		return errors.New("用法: bluebook search <query|trending|suggestions>")
	}
	switch args[0] {
	case "trending":
		return get(c, "/search/trending"+query(args[1:]))
	case "suggestions":
		if len(args) != 2 {
			return errors.New("用法: bluebook search suggestions <query>")
		}
		return get(c, "/search/suggestions?q="+url.QueryEscape(args[1]))
	default:
		path := "/search?q=" + url.QueryEscape(args[0])
		if len(args) > 1 {
			path += "&" + strings.TrimPrefix(query(args[1:]), "?")
		}
		return get(c, path)
	}
}

func runMedia(c *client, args []string) error {
	if len(args) != 2 || args[0] != "upload" {
		return errors.New("用法: bluebook media upload <file>")
	}
	file, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer file.Close()
	contentType := mime.TypeByExtension(filepath.Ext(args[1]))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var reqBody bytes.Buffer
	if err := json.MarshalWrite(&reqBody, map[string]string{"content_type": contentType}); err != nil {
		return err
	}
	payload, err := c.request(http.MethodPost, "/media/presign", reqBody.Bytes(), "application/json")
	if err != nil {
		return err
	}

	var result struct {
		Data struct {
			UploadURL string `json:"upload_url"`
			ObjectKey string `json:"object_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return err
	}
	if result.Data.UploadURL == "" || result.Data.ObjectKey == "" {
		return errors.New("预签名响应不完整")
	}

	upload, err := http.NewRequest(http.MethodPut, result.Data.UploadURL, file)
	if err != nil {
		return err
	}
	upload.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(upload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("媒体上传失败: %s", resp.Status)
	}

	var respBody bytes.Buffer
	if err := json.MarshalWrite(&respBody, map[string]string{"object_key": result.Data.ObjectKey, "content_type": contentType}); err != nil {
		return err
	}
	return writeResponse(respBody.Bytes())
}

func query(args []string) string {
	if len(args) == 0 {
		return ""
	}
	values := url.Values{}
	for i := 0; i < len(args); i++ {
		part := strings.TrimPrefix(args[i], "--")
		if strings.Contains(part, "=") {
			pieces := strings.SplitN(part, "=", 2)
			values.Add(pieces[0], pieces[1])
			continue
		}
		if strings.HasPrefix(args[i], "--") && i+1 < len(args) {
			values.Add(part, args[i+1])
			i++
		}
	}
	if encoded := values.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

const usage = `bluebook is a JSON-first CLI for Blue Book and AI agents.

Configuration:
  BLUEBOOK_API_KEY=bbk_...             delegated API key
  BLUEBOOK_API_URL=http://host/api/v1  optional API base URL

Commands:
  post create|list|get|update|delete    publish and manage posts
  comment create|list|delete            create and browse comments
  media upload <file>                   upload media and return its object key
  like|unlike|collect|uncollect <id>    interact with a post
  follow|unfollow <user-id>             manage follows
  me | feed | search                    browse account and discovery data
  api <method> <path> [--data JSON]     call any Blue Book REST endpoint

Examples:
  bluebook post create --title "Hello" --content "World" --tag ai
  bluebook media upload image.png
  bluebook comment create --post POST_ID --content "Useful"
  bluebook api get /me/collections
`

func printUsage() { fmt.Print(usage) }
