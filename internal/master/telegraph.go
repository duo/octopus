package master

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/duo/octopus/internal/common"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"

	log "github.com/sirupsen/logrus"
)

const (
	createPageURL = "https://api.telegra.ph/createPage"
	uploadURL     = "https://telegra.ph/upload"
)

var (
	once   sync.Once
	client *http.Client
)

type uploadResult struct {
	Source []source
}

type source struct {
	Src string `json:"src"`
}

type uploadError struct {
	Error string `json:"error"`
}

type Node any

type NodeElement struct {
	Tag      string            `json:"tag"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Children []Node            `json:"children,omitempty"`
}

type APIResponse struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type APIResponsePage struct {
	APIResponse
	Result Page `json:"result,omitempty"`
}

type Page struct {
	Path        string `json:"path"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	AuthorName  string `json:"author_name,omitempty"`
	AuthorURL   string `json:"author_url,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	Content     []Node `json:"content,omitempty"`
	Views       int    `json:"views"`
	CanEdit     bool   `json:"can_edit,omitempty"`
}

func (ms *MasterService) postApp(app *common.AppData) (*Page, error) {
	client = getClient(ms.config.Master.Telegraph.Proxy)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(app.Content))
	if err != nil {
		return nil, err
	}

	return createPage(
		doc.Contents(),
		ms.config.Master.Telegraph.Tokens[0],
		app.Title,
		app.Blobs,
	)
}

func createPage(selections *goquery.Selection, token string, title string, blobs map[string]*common.BlobData) (page *Page, err error) {
	nodes := traverseNodes(selections, token, title, blobs)

	params := map[string]interface{}{
		"access_token": token,
		"title":        title,
		"content":      castNodes(nodes),
	}

	var data []byte
	if data, err = apiPost(createPageURL, params); err == nil {
		var res APIResponsePage
		if err = json.Unmarshal(data, &res); err == nil {
			if res.Ok {
				return &res.Result, nil
			}

			err = fmt.Errorf(res.Error)

			log.Warnf("API error from %s (%+v): %v", createPageURL, params, err)
		}
	}

	return &Page{}, err
}

func castNodes(nodes []Node) []interface{} {
	castNodes := []interface{}{}

	for _, node := range nodes {
		switch node.(type) {
		case NodeElement:
			castNodes = append(castNodes, node)
		default:
			if cast, ok := node.(string); ok {
				castNodes = append(castNodes, cast)
			} else {
				log.Warnf("param casting error: %#+v", node)
			}
		}
	}

	return castNodes
}

func traverseNodes(selections *goquery.Selection, token, title string, blobs map[string]*common.BlobData) []Node {
	nodes := []Node{}

	var tag string
	var attrs map[string]string
	var element NodeElement

	selections.Each(func(_ int, child *goquery.Selection) {
		for _, node := range child.Nodes {
			switch node.Type {
			case html.TextNode:
				nodes = append(nodes, node.Data)
			case html.ElementNode:
				attrs = map[string]string{}
				for _, attr := range node.Attr {
					attrs[attr.Key] = attr.Val
				}

				if node.Data == "blockquote" {
					if page, err := createPage(child.Contents(), token, fmt.Sprintf("%s-%s", title, common.NextRandom()), blobs); err == nil {
						element = NodeElement{
							Tag:      "a",
							Attrs:    map[string]string{"href": page.URL},
							Children: []Node{page.Title},
						}
						nodes = append(nodes, element)
						continue
					} else {
						element = NodeElement{
							Tag:      node.Data,
							Attrs:    attrs,
							Children: []Node{page.Title},
						}
						nodes = append(nodes, element)
						continue
					}
				}

				if node.Data == "img" && strings.HasPrefix(attrs["src"], common.REMOTE_PREFIX) {
					parts := strings.Split(attrs["src"], common.REMOTE_PREFIX)
					if len(parts) == 2 {
						if url, err := upload(client, blobs[parts[1]]); err == nil {
							attrs["src"] = url
							element = NodeElement{
								Tag:   node.Data,
								Attrs: attrs,
							}
							nodes = append(nodes, element)
							continue
						} else {
							log.Errorf("Failed to upload image to telegra.ph: %v", err)
						}
					}

					nodes = append(nodes, "[图片]")
					continue
				}

				if len(node.Namespace) > 0 {
					tag = fmt.Sprintf("%s.%s", node.Namespace, node.Data)
				} else {
					tag = node.Data
					if node.Data == "ul" || node.Data == "li" {
						tag = "p"
					} else {
						tag = node.Data
					}
				}
				element = NodeElement{
					Tag:      tag,
					Attrs:    attrs,
					Children: traverseNodes(child.Contents(), token, title, blobs),
				}

				nodes = append(nodes, element)
			default:
				continue
			}
		}
	})

	return nodes
}

func upload(c *http.Client, blob *common.BlobData) (string, error) {
	if blob == nil {
		return "", errors.New("blob not found")
	}

	b := &bytes.Buffer{}
	w := multipart.NewWriter(b)

	part, err := w.CreateFormFile("file", blob.Name)
	if err != nil {
		return "", err
	}
	part.Write(blob.Binary)
	w.Close()

	r, err := http.NewRequest("POST", uploadURL, bytes.NewReader(b.Bytes()))
	if err != nil {
		return "", err
	}
	r.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.Do(r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var jsonData uploadResult
	json.Unmarshal(content, &jsonData.Source)
	if jsonData.Source == nil {
		var err uploadError
		json.Unmarshal(content, &err)
		return "", fmt.Errorf(err.Error)
	}
	return jsonData.Source[0].Src, err
}

func apiPost(apiURL string, params map[string]interface{}) (data []byte, err error) {
	var js []byte
	paramValues := url.Values{}
	for key, value := range params {
		switch v := value.(type) {
		case string:
			paramValues[key] = []string{v}
		default:
			if js, err = json.Marshal(v); err == nil {
				paramValues[key] = []string{string(js)}
			} else {
				log.Warnf("param marshalling error for: %s (%v)", key, err)
				return []byte{}, err
			}
		}
	}
	encoded := paramValues.Encode()

	var req *http.Request
	if req, err = http.NewRequest("POST", apiURL, bytes.NewBufferString(encoded)); err == nil {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(len(encoded)))

		var res *http.Response
		res, err = client.Do(req)

		if err == nil {
			defer res.Body.Close()
			if data, err = io.ReadAll(res.Body); err == nil {
				return data, nil
			}

			log.Warnf("response read error: %v", err)
		} else {
			log.Warnf("request error: %v", err)
		}
	} else {
		log.Warnf("building request error: %v", err)
	}

	return []byte{}, err
}

func getClient(proxy string) *http.Client {
	once.Do(func() {
		client = &http.Client{}

		if proxy != "" {
			proxyUrl, err := url.Parse(proxy)
			if err != nil {
				log.Fatal(err)
			}
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
		}
	})
	return client
}
