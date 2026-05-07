package client

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"mymind/pkg/auth"
	mymindhttp "mymind/pkg/http"
)

// MyMindObject represents a saved object.
type MyMindObject struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type,omitempty"`
	Title     string                 `json:"title,omitempty"`
	URL       string                 `json:"url,omitempty"`
	Summary   string                 `json:"summary,omitempty"`
	Created   string                 `json:"created,omitempty"`
	Modified  string                 `json:"modified,omitempty"`
	Bumped    string                 `json:"bumped,omitempty"`
	Deleted   interface{}            `json:"deleted,omitempty"`
	Tags      []map[string]string    `json:"tags,omitempty"`
	Spaces    []map[string]string    `json:"spaces,omitempty"`
	Notes     []map[string]interface{} `json:"notes,omitempty"`
	Source    map[string]string      `json:"source,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
	Blob      map[string]interface{} `json:"blob,omitempty"`
}

// MyMindSpace represents a space.
type MyMindSpace struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Color   string                 `json:"color,omitempty"`
	Created string                 `json:"created,omitempty"`
	Objects []map[string]string    `json:"objects,omitempty"`
}

// MyMindTag represents a tag.
type MyMindTag struct {
	Name     string `json:"name"`
	Count    int    `json:"count,omitempty"`
	Modified string `json:"modified,omitempty"`
}

// SearchMatch is a search result item.
type SearchMatch struct {
	ID            string  `json:"id"`
	Score         float64 `json:"score"`
	SemanticScore float64 `json:"semanticScore,omitempty"`
}

// SearchResponse is the search endpoint response.
type SearchResponse struct {
	Matches []SearchMatch `json:"matches"`
}

// Client is the MyMind API client.
type Client struct {
	http *mymindhttp.Client
}

// New creates a new MyMind API client.
func New(creds *auth.Credentials) *Client {
	return &Client{http: mymindhttp.New(creds)}
}

// SetVerbose enables verbose HTTP logging to stderr.
func (c *Client) SetVerbose(v bool) { c.http.Verbose = v }

// AuthProbe is a cheap call to verify credentials.
func (c *Client) AuthProbe(opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("GET", "/tags", mymindhttp.RequestOptions{Query: map[string]mymindhttp.QueryValue{"limit": 1}, DryRun: opts.DryRun})
}

// ─── Objects ──────────────────────────────────────────────────────────────────

// ListObjects returns objects, optionally filtered.
func (c *Client) ListObjects(args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	q := map[string]mymindhttp.QueryValue{}
	if v := args["q"]; v != nil {
		q["q"] = v
	}
	if v := args["spaceId"]; v != nil {
		q["spaceId"] = v
	}
	if v := args["similarTo"]; v != nil {
		q["similarTo"] = v
	}
	if v := args["ids"]; v != nil {
		q["id"] = v
	}
	if v := args["limit"]; v != nil {
		q["limit"] = v
	}
	if v := args["contentAs"]; v != nil {
		q["contentAs"] = v
	}
	return c.http.Request("GET", "/objects", mymindhttp.RequestOptions{Query: q, DryRun: opts.DryRun})
}

// GetObject returns a single object by ID.
func (c *Client) GetObject(id string, args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	q := map[string]mymindhttp.QueryValue{}
	if v := args["contentAs"]; v != nil {
		q["contentAs"] = v
	}
	return c.http.Request("GET", "/objects/"+id, mymindhttp.RequestOptions{Query: q, DryRun: opts.DryRun})
}

// CreateObject creates a new object.
func (c *Client) CreateObject(args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	// Check if blob upload
	if blobData, ok := args["blob"].([]byte); ok {
		filename, _ := args["filename"].(string)
		return c.uploadBlob(blobData, filename, args, opts)
	}
	return c.http.Request("POST", "/objects", mymindhttp.RequestOptions{
		Body:        buildCreateBody(args),
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

func buildCreateBody(args map[string]interface{}) string {
	body := map[string]interface{}{}
	if v := args["url"]; v != nil {
		body["url"] = v
	}
	if v := args["content"]; v != nil {
		body["content"] = v
	}
	if v := args["title"]; v != nil {
		body["title"] = v
	}
	if v := args["tags"]; v != nil {
		tags := v.([]string)
		tagObjs := make([]map[string]string, len(tags))
		for i, t := range tags {
			tagObjs[i] = map[string]string{"name": t}
		}
		body["tags"] = tagObjs
	}
	if v := args["spaces"]; v != nil {
		spaces := v.([]string)
		spaceObjs := make([]map[string]string, len(spaces))
		for i, s := range spaces {
			spaceObjs[i] = map[string]string{"id": s}
		}
		body["spaces"] = spaceObjs
	}
	data, _ := json.Marshal(body)
	return string(data)
}

func (c *Client) uploadBlob(data []byte, filename string, args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// metadata part
	metadata := map[string]interface{}{}
	if v := args["title"]; v != nil {
		metadata["title"] = v
	}
	if v := args["tags"]; v != nil {
		tags := v.([]string)
		tagObjs := make([]map[string]string, len(tags))
		for i, t := range tags {
			tagObjs[i] = map[string]string{"name": t}
		}
		metadata["tags"] = tagObjs
	}
	if v := args["spaces"]; v != nil {
		spaces := v.([]string)
		spaceObjs := make([]map[string]string, len(spaces))
		for i, s := range spaces {
			spaceObjs[i] = map[string]string{"id": s}
		}
		metadata["spaces"] = spaceObjs
	}
	metaJSON, _ := json.Marshal(metadata)
	part1, _ := writer.CreateFormFile("metadata", "")
	part1.Write(metaJSON)

	// blob part
	part2, _ := writer.CreateFormFile("blob", filename)
	part2.Write(data)

	writer.Close()
	return c.http.Request("POST", "/objects", mymindhttp.RequestOptions{
		Body:        body.Bytes(),
		ContentType: writer.FormDataContentType(),
		DryRun:      opts.DryRun,
	})
}

// UpdateObject updates an object's title or summary.
func (c *Client) UpdateObject(id string, args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	body := map[string]interface{}{}
	if v := args["title"]; v != nil {
		body["title"] = v
	}
	if v := args["summary"]; v != nil {
		body["summary"] = v
	}
	data, _ := json.Marshal(body)
	return c.http.Request("PATCH", "/objects/"+id, mymindhttp.RequestOptions{
		Body:        string(data),
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// DeleteObject soft-deletes an object.
func (c *Client) DeleteObject(id string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("DELETE", "/objects/"+id, opts)
}

// RestoreObject restores a soft-deleted object.
func (c *Client) RestoreObject(id string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("POST", "/objects/"+id+"/restore", opts)
}

// PinObject pins an object.
func (c *Client) PinObject(id string, args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	var bodyStr string
	if pos, ok := args["position"].(int); ok {
		data, _ := json.Marshal(map[string]int{"position": pos})
		bodyStr = string(data)
	}
	return c.http.Request("POST", "/objects/"+id+"/pin", mymindhttp.RequestOptions{
		Body:        bodyStr,
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// UnpinObject unpins an object.
func (c *Client) UnpinObject(id string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("DELETE", "/objects/"+id+"/pin", opts)
}

// GetObjectContent fetches the content body of an object.
func (c *Client) GetObjectContent(id string, format string, opts mymindhttp.RequestOptions) (interface{}, error) {
	accept := acceptForFormat(format)
	return c.http.Request("GET", "/objects/"+id+"/content", mymindhttp.RequestOptions{
		Accept: accept,
		DryRun: opts.DryRun,
	})
}

func acceptForFormat(format string) string {
	switch format {
	case "markdown":
		return "text/markdown"
	case "html":
		return "text/html"
	case "prose":
		return "application/prose+json"
	default:
		return "text/markdown"
	}
}

// SetObjectContent replaces the content body of an object.
func (c *Client) SetObjectContent(id string, body, format string, opts mymindhttp.RequestOptions) (interface{}, error) {
	ct := acceptForFormat(format)
	return c.http.Request("PUT", "/objects/"+id+"/content", mymindhttp.RequestOptions{
		Body:        body,
		ContentType: ct,
		DryRun:      opts.DryRun,
	})
}

// GetObjectBlob returns a raw response for streaming.
func (c *Client) GetObjectBlob(id string, opts mymindhttp.RequestOptions) (*mymindhttp.DryRunResult, *http.Response, error) {
	return c.http.RequestRaw("GET", "/objects/"+id+"/blob", opts)
}

// GetObjectScreenshot returns a raw response for streaming.
func (c *Client) GetObjectScreenshot(id string, opts mymindhttp.RequestOptions) (*mymindhttp.DryRunResult, *http.Response, error) {
	return c.http.RequestRaw("GET", "/objects/"+id+"/screenshot", opts)
}

// GetObjectThumbnail returns a raw response for streaming.
func (c *Client) GetObjectThumbnail(id string, size string, opts mymindhttp.RequestOptions) (*mymindhttp.DryRunResult, *http.Response, error) {
	q := map[string]mymindhttp.QueryValue{}
	if size != "" {
		q["size"] = size
	}
	return c.http.RequestRaw("GET", "/objects/"+id+"/thumbnail", mymindhttp.RequestOptions{Query: q, DryRun: opts.DryRun})
}

// AddObjectTags adds tags to an object.
func (c *Client) AddObjectTags(id string, tagNames []string, opts mymindhttp.RequestOptions) (interface{}, error) {
	tags := make([]map[string]string, len(tagNames))
	for i, t := range tagNames {
		tags[i] = map[string]string{"name": t}
	}
	body, _ := json.Marshal(map[string]interface{}{"tags": tags})
	return c.http.Request("POST", "/objects/"+id+"/tags", mymindhttp.RequestOptions{
		Body:        string(body),
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// RemoveObjectTags removes tags from an object.
func (c *Client) RemoveObjectTags(id string, tagNames []string, opts mymindhttp.RequestOptions) (interface{}, error) {
	tags := make([]map[string]string, len(tagNames))
	for i, t := range tagNames {
		tags[i] = map[string]string{"name": t}
	}
	body, _ := json.Marshal(tags)
	return c.http.Request("DELETE", "/objects/"+id+"/tags", mymindhttp.RequestOptions{
		Body:        string(body),
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// AddObjectNote adds a note to an object.
func (c *Client) AddObjectNote(id string, body, format string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("POST", "/objects/"+id+"/notes", mymindhttp.RequestOptions{
		Body:        body,
		ContentType: acceptForFormat(format),
		DryRun:      opts.DryRun,
	})
}

// UpdateObjectNote updates an existing note.
func (c *Client) UpdateObjectNote(id, noteID, body, format string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("PUT", "/objects/"+id+"/notes/"+noteID, mymindhttp.RequestOptions{
		Body:        body,
		ContentType: acceptForFormat(format),
		DryRun:      opts.DryRun,
	})
}

// DeleteObjectNote deletes a note.
func (c *Client) DeleteObjectNote(id, noteID string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("DELETE", "/objects/"+id+"/notes/"+noteID, opts)
}

// ─── Spaces ───────────────────────────────────────────────────────────────────

// ListSpaces returns all spaces.
func (c *Client) ListSpaces(opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("GET", "/spaces", opts)
}

// GetSpace returns a single space.
func (c *Client) GetSpace(id string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("GET", "/spaces/"+id, opts)
}

// CreateSpace creates a new space.
func (c *Client) CreateSpace(args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	body, _ := json.Marshal(args)
	return c.http.Request("POST", "/spaces", mymindhttp.RequestOptions{
		Body:        string(body),
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// UpdateSpace updates a space's name or color.
func (c *Client) UpdateSpace(id string, args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	body, _ := json.Marshal(args)
	return c.http.Request("PATCH", "/spaces/"+id, mymindhttp.RequestOptions{
		Body:        string(body),
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// DeleteSpace deletes a space.
func (c *Client) DeleteSpace(id string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("DELETE", "/spaces/"+id, opts)
}

// AttachToSpace attaches an object to a space.
func (c *Client) AttachToSpace(spaceID, objectID string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("PUT", "/spaces/"+spaceID+"/objects/"+objectID, opts)
}

// DetachFromSpace detaches an object from a space.
func (c *Client) DetachFromSpace(spaceID, objectID string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("DELETE", "/spaces/"+spaceID+"/objects/"+objectID, opts)
}

// ListTags returns all tags.
func (c *Client) ListTags(limit int, opts mymindhttp.RequestOptions) (interface{}, error) {
	q := map[string]mymindhttp.QueryValue{}
	if limit > 0 {
		q["limit"] = limit
	}
	return c.http.Request("GET", "/tags", mymindhttp.RequestOptions{Query: q, DryRun: opts.DryRun})
}

// ─── Search ───────────────────────────────────────────────────────────────────

// Search performs a search query.
func (c *Client) Search(args map[string]interface{}, opts mymindhttp.RequestOptions) (interface{}, error) {
	q := map[string]mymindhttp.QueryValue{}
	if v := args["q"]; v != nil {
		q["q"] = v
	}
	if v := args["limit"]; v != nil {
		q["limit"] = v
	}
	if v := args["semantic"]; v != nil {
		q["semantic"] = v
	}
	if v := args["semanticBoost"]; v != nil {
		q["semanticBoost"] = v
	}
	if v := args["similarTo"]; v != nil {
		q["similarTo"] = v
	}
	if v := args["rerank"]; v != nil {
		q["rerank"] = v
	}
	return c.http.Request("GET", "/search", mymindhttp.RequestOptions{Query: q, DryRun: opts.DryRun})
}

// ─── Convert ──────────────────────────────────────────────────────────────────

// Convert converts content between formats.
func (c *Client) Convert(input, from, to string, opts mymindhttp.RequestOptions) (interface{}, error) {
	ct := acceptForFormat(from)
	accept := acceptForFormat(to)
	return c.http.Request("POST", "/convert", mymindhttp.RequestOptions{
		Body:        input,
		ContentType: ct,
		Accept:      accept,
		DryRun:      opts.DryRun,
	})
}

// ─── Tags ─────────────────────────────────────────────────────────────────────

// GetTag returns a single tag.
func (c *Client) GetTag(name string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("GET", "/tags/"+name, opts)
}

// CreateTag creates a new tag.
func (c *Client) CreateTag(name string, opts mymindhttp.RequestOptions) (interface{}, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	return c.http.Request("POST", "/tags", mymindhttp.RequestOptions{
		Body:        string(body),
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// DeleteTag deletes a tag.
func (c *Client) DeleteTag(name string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("DELETE", "/tags/"+name, opts)
}

// ─── Spaces (members) ─────────────────────────────────────────────────────────

// AddSpaceMembers adds members to a space.
func (c *Client) AddSpaceMembers(id string, memberIDs []string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("POST", "/spaces/"+id+"/members", mymindhttp.RequestOptions{
		Body:        `{"memberIds":["` + strings.Join(memberIDs, `","`) + `"]}`,
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// RemoveSpaceMembers removes members from a space.
func (c *Client) RemoveSpaceMembers(id string, memberIDs []string, opts mymindhttp.RequestOptions) (interface{}, error) {
	return c.http.Request("DELETE", "/spaces/"+id+"/members", mymindhttp.RequestOptions{
		Body:        `{"memberIds":["` + strings.Join(memberIDs, `","`) + `"]}`,
		ContentType: "application/json",
		DryRun:      opts.DryRun,
	})
}

// ─── StreamToFile ─────────────────────────────────────────────────────────────

// StreamToFile copies a response body to a file.
func StreamToFile(r io.Reader, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
