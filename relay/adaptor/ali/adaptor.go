package ali

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/labring/aiproxy/model"
	"github.com/labring/aiproxy/relay/adaptor/openai"
	"github.com/labring/aiproxy/relay/meta"
	"github.com/labring/aiproxy/relay/mode"
	relaymodel "github.com/labring/aiproxy/relay/model"
	"github.com/labring/aiproxy/relay/utils"
)

// https://help.aliyun.com/zh/dashscope/developer-reference/api-details

type Adaptor struct{}

const baseURL = "https://dashscope.aliyuncs.com"

func (a *Adaptor) GetBaseURL() string {
	return baseURL
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	u := meta.Channel.BaseURL
	if u == "" {
		u = baseURL
	}
	switch meta.Mode {
	case mode.ImagesGenerations:
		return u + "/api/v1/services/aigc/text2image/image-synthesis", nil
	case mode.ChatCompletions:
		if meta.ActualModel == "farui-plus" {
			return u + "/api/v1/services/aigc/text-generation/generation", nil
		}
		return u + "/compatible-mode/v1/chat/completions", nil
	case mode.Completions:
		return u + "/compatible-mode/v1/completions", nil
	case mode.Embeddings:
		return u + "/compatible-mode/v1/embeddings", nil
	case mode.AudioSpeech, mode.AudioTranscription:
		return u + "/api-ws/v1/inference", nil
	case mode.Rerank:
		return u + "/api/v1/services/rerank/text-rerank/text-rerank", nil
	default:
		return "", fmt.Errorf("unsupported mode: %s", meta.Mode)
	}
}

func (a *Adaptor) SetupRequestHeader(meta *meta.Meta, _ *gin.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+meta.Channel.Key)

	// req.Header.Set("X-Dashscope-Plugin", meta.Channel.Config.Plugin)
	return nil
}

func (a *Adaptor) ConvertRequest(meta *meta.Meta, req *http.Request) (string, http.Header, io.Reader, error) {
	switch meta.Mode {
	case mode.ImagesGenerations:
		return ConvertImageRequest(meta, req)
	case mode.Rerank:
		return ConvertRerankRequest(meta, req)
	case mode.ChatCompletions, mode.Completions, mode.Embeddings:
		return openai.ConvertRequest(meta, req)
	case mode.AudioSpeech:
		return ConvertTTSRequest(meta, req)
	case mode.AudioTranscription:
		return ConvertSTTRequest(meta, req)
	default:
		return "", nil, nil, fmt.Errorf("unsupported mode: %s", meta.Mode)
	}
}

func (a *Adaptor) DoRequest(meta *meta.Meta, _ *gin.Context, req *http.Request) (*http.Response, error) {
	switch meta.Mode {
	case mode.AudioSpeech:
		return TTSDoRequest(meta, req)
	case mode.AudioTranscription:
		return STTDoRequest(meta, req)
	case mode.ChatCompletions:
		fallthrough
	default:
		return utils.DoRequest(req)
	}
}

func (a *Adaptor) DoResponse(meta *meta.Meta, c *gin.Context, resp *http.Response) (usage *relaymodel.Usage, err *relaymodel.ErrorWithStatusCode) {
	switch meta.Mode {
	case mode.ImagesGenerations:
		usage, err = ImageHandler(meta, c, resp)
	case mode.ChatCompletions, mode.Completions, mode.Embeddings:
		if meta.ActualModel == "farui-plus" {
			usage, err = DoFaruiResponse(meta, c, resp)
		} else {
			usage, err = openai.DoResponse(meta, c, resp)
		}
	case mode.Rerank:
		usage, err = RerankHandler(meta, c, resp)
	case mode.AudioSpeech:
		usage, err = TTSDoResponse(meta, c, resp)
	case mode.AudioTranscription:
		usage, err = STTDoResponse(meta, c, resp)
	default:
		return nil, openai.ErrorWrapperWithMessage(fmt.Sprintf("unsupported mode: %s", meta.Mode), "unsupported_mode", http.StatusBadRequest)
	}
	return
}

func (a *Adaptor) GetModelList() []*model.ModelConfig {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "ali"
}
