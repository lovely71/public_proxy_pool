package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/subcheck"
	"github.com/qiyiyun/public_proxy_pool/internal/validator"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	defaultTimeout := cfg.ValidateTimeout
	if defaultTimeout <= 0 {
		defaultTimeout = 8 * time.Second
	}
	defaultSingBoxPath := strings.TrimSpace(cfg.SingBoxPath)
	if defaultSingBoxPath == "" {
		defaultSingBoxPath = "sing-box"
	}

	subscriptionURL := flag.String("url", "", "项目生成的订阅链接")
	apiKey := flag.String("api-key", "", "可选，请求订阅链接时附带的 X-API-Key")
	format := flag.String("format", subcheck.FormatAuto, "订阅格式: auto|plain|v2ray|clash")
	huggingFaceURL := flag.String("huggingface-url", validator.DefaultHuggingFaceURL, "Hugging Face 检测地址")
	targetURL := flag.String("target-url", validator.DefaultCheckIPURL, "Cloudflare IP 检测地址")
	timeout := flag.Duration("timeout", defaultTimeout, "单节点检测超时")
	fetchTimeout := flag.Duration("fetch-timeout", 15*time.Second, "拉取订阅内容的超时")
	concurrency := flag.Int("concurrency", 8, "并发检测数量")
	v2rayMode := flag.String("v2ray-mode", "sing-box", "v2ray 节点检测模式，当前建议 sing-box")
	singBoxPath := flag.String("sing-box-path", defaultSingBoxPath, "sing-box 可执行文件路径")
	jsonOutput := flag.Bool("json", false, "以 JSON 输出检测结果")
	flag.Parse()

	if strings.TrimSpace(*subscriptionURL) == "" {
		flag.Usage()
		os.Exit(2)
	}

	cfg.ValidateTimeout = *timeout
	cfg.V2RayValidateMode = strings.TrimSpace(*v2rayMode)
	cfg.SingBoxPath = strings.TrimSpace(*singBoxPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	body, contentType, err := subcheck.FetchSubscription(ctx, *subscriptionURL, *apiKey, *fetchTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch subscription failed: %v\n", err)
		os.Exit(1)
	}

	nodes, detectedFormat, err := subcheck.ParseSubscriptionNodes(body, *subscriptionURL, contentType, *format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse subscription failed: %v\n", err)
		os.Exit(1)
	}

	results := subcheck.CheckNodes(ctx, cfg, nodes, *huggingFaceURL, *targetURL, *concurrency)
	counts := subcheck.TierSummary(results)

	if *jsonOutput {
		payload := map[string]any{
			"subscription_url": *subscriptionURL,
			"format":           detectedFormat,
			"target_url":       strings.TrimSpace(*targetURL),
			"total":            counts.Total,
			"alive":            counts.Alive,
			"google_ok":        counts.GoogleOK,
			"huggingface_ok":   counts.HuggingFaceOK,
			"checkip_ok":       counts.CheckIPOK,
			"ok":               counts.CheckIPOK,
			"fail":             counts.Fail,
			"results":          results,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintf(os.Stderr, "encode json failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Subscription: %s\n", *subscriptionURL)
		fmt.Printf("Format: %s\n", detectedFormat)
		fmt.Printf("Hugging Face: %s\n", strings.TrimSpace(*huggingFaceURL))
		fmt.Printf("Target: %s\n", strings.TrimSpace(*targetURL))
		fmt.Printf("Concurrency: %d\n", *concurrency)
		fmt.Printf("Nodes: %d\n\n", counts.Total)

		for _, res := range results {
			fmt.Printf("[A:%s G:%s H:%s C:%s] #%03d %-7s %-24s endpoint=%-24s g204=%-3d/%-5dms hf=%-3d/%-5dms checkip=%-3d/%-5dms ip=%-39s",
				yn(res.Alive), yn(res.GoogleOK), yn(res.HuggingFaceOK), yn(res.CheckIPOK),
				res.Index, res.Protocol, truncate(res.Name, 24), truncate(res.Endpoint, 24),
				res.GoogleStatus, res.GoogleLatency, res.HFStatus, res.HFLatency, res.HTTPStatus, res.LatencyMS, truncate(res.ExitIP, 39))
			if res.Error != "" {
				fmt.Printf(" err=%s", res.Error)
			}
			fmt.Println()
		}

		fmt.Printf("\nSummary: total=%d alive=%d google_ok=%d huggingface_ok=%d checkip_ok=%d fail=%d\n",
			counts.Total, counts.Alive, counts.GoogleOK, counts.HuggingFaceOK, counts.CheckIPOK, counts.Fail)
	}

	if counts.CheckIPOK == 0 {
		os.Exit(1)
	}
}

func truncate(s string, width int) string {
	s = strings.TrimSpace(s)
	if width <= 0 || len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

func yn(ok bool) string {
	if ok {
		return "Y"
	}
	return "N"
}
