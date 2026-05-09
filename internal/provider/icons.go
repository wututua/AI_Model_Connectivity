package provider

import "strings"

var providerIcons = map[string]string{
	"openai":             "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/openai.svg",
	"azure":              "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/azure.svg",
	"xai":                "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/xai.svg",
	"anthropic":          "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/anthropic.svg",
	"ollama":             "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/ollama.svg",
	"google":             "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/gemini-color.svg",
	"deepseek":           "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/deepseek.svg",
	"modelscope":         "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/modelscope.svg",
	"zhipu":              "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/zhipu.svg",
	"nvidia":             "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/nvidia-color.svg",
	"siliconflow":        "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/siliconcloud.svg",
	"moonshot":           "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi.svg",
	"kimi":               "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi.svg",
	"kimi-code":          "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi.svg",
	"longcat":            "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/longcat-color.svg",
	"ppio":               "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/ppio.svg",
	"dify":               "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/dify-color.svg",
	"coze":               "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@1.66.0/icons/coze.svg",
	"dashscope":          "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/alibabacloud-color.svg",
	"deerflow":           "https://cdn.jsdelivr.net/gh/bytedance/deer-flow@main/frontend/public/images/deer.svg",
	"fastgpt":            "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/fastgpt-color.svg",
	"lm_studio":          "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/lmstudio.svg",
	"fishaudio":          "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/fishaudio.svg",
	"minimax":            "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/minimax.svg",
	"minimax-token-plan": "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/minimax.svg",
	"mimo":               "https://platform.xiaomimimo.com/favicon.874c9507.png",
	"302ai":              "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@1.53.0/icons/ai302-color.svg",
	"microsoft":          "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/microsoft.svg",
	"vllm":               "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/vllm.svg",
	"groq":               "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/groq.svg",
	"aihubmix":           "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/aihubmix-color.svg",
	"openrouter":         "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/openrouter.svg",
	"tokenpony":          "https://tokenpony.cn/tokenpony-web/logo.png",
	"compshare":          "https://compshare.cn/favicon.ico",
	"xinference":         "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/xinference-color.svg",
	"bailian":            "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/bailian-color.svg",
	"volcengine":         "https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/volcengine-color.svg",
}

func IconFor(id, providerType, name string) string {
	keys := []string{strings.ToLower(id), strings.ToLower(providerType), strings.ToLower(name)}
	for _, key := range keys {
		if icon := providerIcons[key]; icon != "" {
			return icon
		}
	}
	for _, key := range keys {
		tokens := strings.FieldsFunc(strings.ReplaceAll(key, "-", "_"), func(r rune) bool { return r == '_' || r == ' ' })
		for iconKey, icon := range providerIcons {
			if strings.HasPrefix(key, iconKey) || contains(tokens, iconKey) {
				return icon
			}
		}
	}
	return ""
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
