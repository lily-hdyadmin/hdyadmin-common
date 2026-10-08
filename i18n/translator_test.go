package i18n_test

import (
	"context"
	"testing"
	"testing/fstest"

	kratosErrors "github.com/go-kratos/kratos/v2/errors"
	commonV1 "github.com/lily-hdyadmin/hdyadmin-common/gen/go/common/service/v1"
	commonI18N "github.com/lily-hdyadmin/hdyadmin-common/i18n"
)

func TestTranslatorUsesCommonCatalog(t *testing.T) {
	translator, err := commonI18N.New(commonI18N.Config{DefaultLanguage: "zh-CN"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	zh := translator.WithLanguage(context.Background(), "zh-CN")
	if got := translator.Trans(zh, commonI18N.CreateSuccess); got != "创建成功" {
		t.Fatalf("translated message = %q", got)
	}
	if got := translator.MatchLanguage("fr-FR;q=0.2,en-US;q=0.9"); got != "en" {
		t.Fatalf("matched language = %q, want en", got)
	}
	if got := translator.MatchLanguage("fr-FR"); got != "zh-CN" {
		t.Fatalf("fallback language = %q, want zh-CN", got)
	}
}

func TestServiceCatalogOverridesAndExtendsCommonCatalog(t *testing.T) {
	serviceCatalog := fstest.MapFS{
		"locale/en.json": &fstest.MapFile{Data: []byte(`{
			"common": {"success": "Service success"},
			"response": {"completed": "Task %s completed"}
		}`)},
		"locale/zh-CN.json": &fstest.MapFile{Data: []byte(`{
			"common": {"success": "服务操作成功"},
			"response": {"completed": "任务 %s 已完成"}
		}`)},
	}
	translator, err := commonI18N.New(
		commonI18N.Config{DefaultLanguage: "zh-CN"},
		commonI18N.CatalogSource{FS: serviceCatalog, Root: "locale"},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	zh := translator.WithLanguage(context.Background(), "zh-CN")
	if got := translator.Trans(zh, commonI18N.Success); got != "服务操作成功" {
		t.Fatalf("overridden message = %q", got)
	}
	if got := translator.Trans(zh, commonI18N.Message("response.completed", "backup")); got != "任务 backup 已完成" {
		t.Fatalf("service message = %q", got)
	}
	if got := translator.Trans(zh, "Task restore completed"); got != "任务 restore 已完成" {
		t.Fatalf("legacy template = %q", got)
	}
}

func TestTranslatorTranslatesErrorAndProtoResponse(t *testing.T) {
	translator, err := commonI18N.New(commonI18N.Config{DefaultLanguage: "zh-CN"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := translator.WithLanguage(context.Background(), "zh-CN")

	original := kratosErrors.New(503, "UPSTREAM_UNAVAILABLE", "private upstream detail").
		WithMetadata(map[string]string{"upstream": "scheduler"})
	translated := kratosErrors.FromError(translator.TransError(ctx, original))
	if translated.Code != 503 || translated.Reason != "UPSTREAM_UNAVAILABLE" {
		t.Fatalf("translated error shape = code %d reason %q", translated.Code, translated.Reason)
	}
	if translated.Message != "服务暂不可用" {
		t.Fatalf("translated error message = %q", translated.Message)
	}
	if translated.Metadata["upstream"] != "scheduler" {
		t.Fatalf("translated metadata = %#v", translated.Metadata)
	}

	response := &commonV1.ExecuteTaskResponse{Message: commonI18N.UpdateSuccess}
	translator.TransResponse(ctx, response)
	if response.GetMessage() != "更新成功" {
		t.Fatalf("translated response message = %q", response.GetMessage())
	}
}
