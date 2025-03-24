package ali

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/labring/aiproxy/common"
	"github.com/labring/aiproxy/common/render"
	"github.com/labring/aiproxy/middleware"
	"github.com/labring/aiproxy/relay/adaptor/openai"
	"github.com/labring/aiproxy/relay/meta"
	model "github.com/labring/aiproxy/relay/model"
	relaymodel "github.com/labring/aiproxy/relay/model"
	"github.com/labring/aiproxy/relay/utils"
)

func ConvertFaruiRequest(meta *meta.Meta, req *http.Request) (string, http.Header, io.Reader, error) {
	var requestBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Model       string  `json:"model"`
		Temperature float64 `json:"temperature,omitempty"`
		TopP        float64 `json:"top_p,omitempty"`
		MaxTokens   int     `json:"max_tokens,omitempty"`
		Stream      bool    `json:"stream,omitempty"`
	}

	if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
		return "", nil, nil, err
	}

	aliRequest := struct {
		Model string `json:"model"`
		Input struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		} `json:"input"`
		Parameters struct {
			ResultFormat string  `json:"result_format"`
			Temperature  float64 `json:"temperature,omitempty"`
			TopP         float64 `json:"top_p,omitempty"`
			MaxTokens    int     `json:"max_tokens,omitempty"`
			Incremental  bool    `json:"incremental,omitempty"`
		} `json:"parameters"`
	}{
		Model: meta.ActualModel,
		Input: struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}{
			Messages: requestBody.Messages,
		},
		Parameters: struct {
			ResultFormat string  `json:"result_format"`
			Temperature  float64 `json:"temperature,omitempty"`
			TopP         float64 `json:"top_p,omitempty"`
			MaxTokens    int     `json:"max_tokens,omitempty"`
			Incremental  bool    `json:"incremental,omitempty"`
		}{
			ResultFormat: "message",
			Temperature:  requestBody.Temperature,
			TopP:         requestBody.TopP,
			MaxTokens:    requestBody.MaxTokens,
			Incremental:  requestBody.Stream,
		},
	}

	jsonData, err := json.Marshal(aliRequest)
	if err != nil {
		return "", nil, nil, err
	}
	req.Header.Set("X-DashScope-SSE", "enable")
	return req.Method, req.Header, bytes.NewReader(jsonData), nil
}

func DoFaruiResponse(meta *meta.Meta, c *gin.Context, resp *http.Response) (usage *relaymodel.Usage, err *relaymodel.ErrorWithStatusCode) {
	if utils.IsStreamResponse(resp) || true {
		usage, err = StreamHandler(meta, c, resp)
	} else {
		// 非流式，暂未实现
		// usage, err = Handler(meta, c, resp, nil)
	}
	return
}

func StreamHandler(meta *meta.Meta, c *gin.Context, resp *http.Response) (*model.Usage, *model.ErrorWithStatusCode) {
	if resp.StatusCode != http.StatusOK {
		return nil, openai.ErrorHanlder(resp)
	}
	defer resp.Body.Close()

	log := middleware.GetLogger(c)

	common.SetEventStreamHeaders(c)

	lastContent := ""
	var usage *model.Usage

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Error("error reading stream: " + err.Error())
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			continue
		}

		line = strings.TrimPrefix(line, "data:")
		if line == "[DONE]" {
			continue
		}
		// 转成 openai 协议
		var openaiResponse any
		var u *model.Usage
		openaiResponse, u, lastContent, err = convertFaruiResponse(lastContent, line)
		if err != nil {
			log.Error("error converting farui response: " + err.Error())
			continue
		}
		if u != nil {
			usage = u // usage 暂时未实现，用不到
		}
		_ = render.ObjectData(c, openaiResponse)
	}
	render.Done(c)

	if usage.TotalTokens != 0 && usage.PromptTokens == 0 { // some channels don't return prompt tokens & completion tokens
		usage.PromptTokens = meta.InputTokens
		usage.CompletionTokens = usage.TotalTokens - meta.InputTokens
	}

	return usage, nil // usage 暂时未实现，用不到
}

func convertFaruiResponse(lastContent string, line string) (any, *model.Usage, string, error) {
	var response struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
					Role    string `json:"role"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			TotalTokens  int `json:"total_tokens"`
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		RequestID string `json:"request_id"`
	}

	if err := json.Unmarshal([]byte(line), &response); err != nil {
		return nil, nil, "", err
	}
	// 计算delta
	runes1 := []rune(response.Output.Choices[0].Message.Content)
	runes2 := []rune(lastContent)
	delta := ""
	for i := range runes1 {
		if i >= len(runes2) {
			delta += string(runes1[i])
			continue
		}
		if runes1[i] == runes2[i] {
			continue
		}
		delta += string(runes1[i])
	}

	openaiResponse := map[string]any{
		"choices": []map[string]any{
			{
				"delta": map[string]any{
					"content": delta,
				},
				"finish_reason": response.Output.Choices[0].FinishReason,
				"index":         0,
				"logprobs":      nil,
			},
		},
		"object":             "chat.completion.chunk",
		"usage":              nil,
		"created":            time.Now().Unix(),
		"system_fingerprint": nil,
		"model":              "farui-plus",
		"id":                 response.RequestID,
	}

	usage := &model.Usage{
		PromptTokens:     response.Usage.InputTokens,
		CompletionTokens: response.Usage.OutputTokens,
		TotalTokens:      response.Usage.TotalTokens,
	}

	return openaiResponse, usage, response.Output.Choices[0].Message.Content, nil
}
