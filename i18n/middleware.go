package i18n

import (
	"context"
	"net/http"
	"reflect"
	"strings"

	kratosErrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	kratosHTTP "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var commonTranslatableFields = map[protoreflect.Name]struct{}{
	"message":       {},
	"msg":           {},
	"error_message": {},
}

var statusMessageIDs = map[int32]string{
	400: "http.badRequest",
	401: "http.unauthorized",
	402: "http.paymentRequired",
	403: "http.forbidden",
	404: "http.notFound",
	405: "http.methodNotAllowed",
	406: "http.notAcceptable",
	407: "http.proxyAuthenticationRequired",
	408: "http.requestTimeout",
	409: "http.conflict",
	410: "http.gone",
	411: "http.lengthRequired",
	412: "http.preconditionFailed",
	413: "http.payloadTooLarge",
	414: "http.uriTooLong",
	415: "http.unsupportedMediaType",
	416: "http.rangeNotSatisfiable",
	417: "http.expectationFailed",
	418: "http.imATeapot",
	421: "http.misdirectedRequest",
	422: "http.unprocessableEntity",
	423: "http.locked",
	424: "http.failedDependency",
	425: "http.tooEarly",
	426: "http.upgradeRequired",
	428: "http.preconditionRequired",
	429: "http.tooManyRequests",
	431: "http.requestHeaderFieldsTooLarge",
	451: "http.unavailableForLegalReasons",
	500: "http.internalServerError",
	501: "http.notImplemented",
	502: "http.badGateway",
	503: "http.serviceUnavailable",
	504: "http.gatewayTimeout",
	505: "http.httpVersionNotSupported",
	506: "http.variantAlsoNegotiates",
	507: "http.insufficientStorage",
	508: "http.loopDetected",
	510: "http.notExtended",
	511: "http.networkAuthenticationRequired",
	598: "http.networkReadTimeout",
	599: "http.networkConnectTimeout",
}

// ServerMiddleware 在 HTTP/gRPC API 边界统一处理语言、响应提示和错误翻译。
func (t *Translator) ServerMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			ctx = t.WithLanguage(ctx, languageFromTransport(ctx))
			if tr, ok := transport.FromServerContext(ctx); ok {
				tr.ReplyHeader().Set("content-language", t.Language(ctx))
			}

			response, err := handler(ctx, req)
			if err != nil {
				return nil, t.TransError(ctx, err)
			}
			t.TransResponse(ctx, response)
			return response, nil
		}
	}
}

// StreamServerMiddleware 在流消息发送前翻译响应提示字段。
func (t *Translator) StreamServerMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, message any) (any, error) {
			ctx = t.WithLanguage(ctx, languageFromTransport(ctx))
			t.TransResponse(ctx, message)
			response, err := handler(ctx, message)
			if err != nil {
				return nil, t.TransError(ctx, err)
			}
			return response, nil
		}
	}
}

// StreamErrorInterceptor 翻译流式 RPC 处理器最终返回的错误。
func (t *Translator) StreamErrorInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := t.WithLanguage(stream.Context(), languageFromTransport(stream.Context()))
		if tr, ok := transport.FromServerContext(ctx); ok {
			tr.ReplyHeader().Set("content-language", t.Language(ctx))
		}
		err := handler(srv, &languageServerStream{ServerStream: stream, ctx: ctx})
		return t.TransError(ctx, err)
	}
}

// TransError 翻译错误消息，同时保留状态码、reason、metadata 和 cause。
func (t *Translator) TransError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	statusErr := kratosErrors.FromError(err)
	localeName := t.Language(ctx)
	message, ok := t.translate(localeName, statusErr.Message)
	if !ok && t.indexForName(localeName) != t.englishIndex() && statusErr.Reason != "" {
		message, ok = t.translate(localeName, "reason."+statusErr.Reason)
	}
	if !ok && t.indexForName(localeName) != t.englishIndex() {
		if id, exists := statusMessageIDs[statusErr.Code]; exists {
			message, ok = t.translate(localeName, id)
		}
	}
	if !ok || message == statusErr.Message {
		return err
	}
	return kratosErrors.New(int(statusErr.Code), statusErr.Reason, message).
		WithMetadata(statusErr.Metadata).
		WithCause(err)
}

// TransResponse 递归翻译 protobuf 响应中的通用错误和提示字段。
// 业务名称、描述等领域字段由具体服务负责生成独立的展示字段，公共层不修改原始数据。
func (t *Translator) TransResponse(ctx context.Context, response any) {
	message, ok := response.(proto.Message)
	if !ok || message == nil {
		return
	}
	value := reflect.ValueOf(message)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return
	}
	t.transProtoMessage(ctx, message.ProtoReflect())
}

// HTTPErrorEncoder 为 Kratos HTTP 路由提供统一错误格式和翻译行为。
func (t *Translator) HTTPErrorEncoder() kratosHTTP.EncodeErrorFunc {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		ctx := t.WithLanguage(r.Context(), languageFromHTTPHeader(r.Header))
		w.Header().Set("Content-Language", t.Language(ctx))
		kratosHTTP.DefaultErrorEncoder(w, r, t.TransError(ctx, err))
	}
}

func (t *Translator) transProtoMessage(ctx context.Context, message protoreflect.Message) {
	if !message.IsValid() {
		return
	}
	fields := message.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.IsList() {
			if field.Kind() == protoreflect.MessageKind {
				list := message.Get(field).List()
				for j := 0; j < list.Len(); j++ {
					t.transProtoMessage(ctx, list.Get(j).Message())
				}
			}
			continue
		}
		if field.IsMap() {
			if field.MapValue().Kind() == protoreflect.MessageKind {
				message.Get(field).Map().Range(func(_ protoreflect.MapKey, value protoreflect.Value) bool {
					t.transProtoMessage(ctx, value.Message())
					return true
				})
			}
			continue
		}
		if field.Kind() == protoreflect.MessageKind {
			if message.Has(field) {
				t.transProtoMessage(ctx, message.Get(field).Message())
			}
			continue
		}
		if field.Kind() != protoreflect.StringKind {
			continue
		}
		if !t.isTranslatableField(field.Name()) || !message.Has(field) {
			continue
		}
		current := message.Get(field).String()
		translated, ok := t.translate(t.Language(ctx), current)
		if ok && translated != current {
			message.Set(field, protoreflect.ValueOfString(translated))
		}
	}
}

func (t *Translator) isTranslatableField(fieldName protoreflect.Name) bool {
	_, ok := commonTranslatableFields[fieldName]
	return ok
}

func languageFromTransport(ctx context.Context) string {
	tr, ok := transport.FromServerContext(ctx)
	if !ok || tr.RequestHeader() == nil {
		return ""
	}
	for _, key := range []string{"accept-language", "x-language", "language", "lang"} {
		if value := strings.TrimSpace(tr.RequestHeader().Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func languageFromHTTPHeader(header http.Header) string {
	for _, key := range []string{"Accept-Language", "X-Language", "Language", "Lang"} {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

type languageServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *languageServerStream) Context() context.Context { return s.ctx }
