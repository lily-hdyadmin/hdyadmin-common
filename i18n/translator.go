package i18n

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/language"
)

const (
	DefaultLanguage = "zh-CN"
	messagePrefix   = "i18n:"
)

var printfDirective = regexp.MustCompile(`%(?:\[[0-9]+\])?[-+#0 ']*[0-9]*(?:\.[0-9]+)?[bcdoOqwxXUeEfFgGspTtv]`)

type contextKey struct{}

// Config 定义默认语言和可选的外部词典目录。
type Config struct {
	DefaultLanguage string `json:"default_language" yaml:"default_language"`
	Dir             string `json:"dir" yaml:"dir"`
}

// CatalogSource 描述由具体服务提供的内嵌词典。
// 后加入的词典会覆盖同语言、同消息键的基础词典。
type CatalogSource struct {
	FS   fs.FS
	Root string
}

type catalog struct {
	name     string
	tag      language.Tag
	messages map[string]string
}

type messageTemplate struct {
	id      string
	pattern *regexp.Regexp
}

// Translator 统一管理词典、语言匹配、消息参数和旧英文消息兼容翻译。
type Translator struct {
	catalogs     []catalog
	matcher      language.Matcher
	defaultIndex int
	sourceToID   map[string]string
	templates    []messageTemplate
}

type encodedMessage struct {
	ID   string   `json:"id"`
	Args []string `json:"args,omitempty"`
}

// Message 创建可延迟翻译的稳定消息键。
// 参数会被安全编码，在 HTTP/gRPC API 边界按请求语言渲染。
func Message(id string, args ...any) string {
	if len(args) == 0 {
		return id
	}
	payload := encodedMessage{ID: id, Args: make([]string, len(args))}
	for i, arg := range args {
		payload.Args[i] = fmt.Sprint(arg)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return id
	}
	return messagePrefix + base64.RawURLEncoding.EncodeToString(data)
}

// New 创建统一翻译器。
// 加载顺序为公共内置词典、服务词典、外部词典目录，后者可覆盖前者。
func New(cfg Config, sources ...CatalogSource) (*Translator, error) {
	if strings.TrimSpace(cfg.DefaultLanguage) == "" {
		cfg.DefaultLanguage = DefaultLanguage
	}

	loaded, err := loadCatalogs(LocaleFS, "locale")
	if err != nil {
		return nil, fmt.Errorf("load common embedded i18n catalogs: %w", err)
	}
	for _, source := range sources {
		if source.FS == nil {
			return nil, fmt.Errorf("load embedded i18n catalogs: catalog filesystem is nil")
		}
		root := source.Root
		if root == "" {
			root = "."
		}
		serviceCatalogs, err := loadCatalogs(source.FS, root)
		if err != nil {
			return nil, fmt.Errorf("load embedded i18n catalogs from %s: %w", root, err)
		}
		mergeCatalogs(loaded, serviceCatalogs)
	}
	if strings.TrimSpace(cfg.Dir) != "" {
		overrides, err := loadCatalogs(os.DirFS(cfg.Dir), ".")
		if err != nil {
			return nil, fmt.Errorf("load i18n catalogs from %s: %w", cfg.Dir, err)
		}
		mergeCatalogs(loaded, overrides)
	}

	names := make([]string, 0, len(loaded))
	for name := range loaded {
		names = append(names, name)
	}
	sort.Strings(names)

	t := &Translator{sourceToID: make(map[string]string)}
	for _, name := range names {
		tag, err := language.Parse(name)
		if err != nil {
			return nil, fmt.Errorf("invalid locale filename %q: %w", name, err)
		}
		t.catalogs = append(t.catalogs, catalog{name: name, tag: tag, messages: loaded[name]})
	}
	if len(t.catalogs) == 0 {
		return nil, fmt.Errorf("no i18n catalogs found")
	}

	tags := make([]language.Tag, len(t.catalogs))
	for i := range t.catalogs {
		tags[i] = t.catalogs[i].tag
	}
	t.matcher = language.NewMatcher(tags)
	_, defaultIndex, confidence := t.matcher.Match(language.Make(cfg.DefaultLanguage))
	t.defaultIndex = defaultIndex
	if confidence == language.No {
		_, t.defaultIndex, _ = t.matcher.Match(language.Make(DefaultLanguage))
	}
	t.buildSourceIndex()
	return t, nil
}

// WithLanguage 将 Accept-Language 等请求语言匹配后写入 context。
func (t *Translator) WithLanguage(ctx context.Context, requested string) context.Context {
	return context.WithValue(ctx, contextKey{}, t.MatchLanguage(requested))
}

// Language 返回 context 中已匹配的语言；没有时返回默认语言。
func (t *Translator) Language(ctx context.Context) string {
	if ctx != nil {
		if value, ok := ctx.Value(contextKey{}).(string); ok && value != "" {
			return value
		}
	}
	return t.catalogs[t.defaultIndex].name
}

// MatchLanguage 按 Accept-Language 的权重选择支持的语言。
func (t *Translator) MatchLanguage(requested string) string {
	if strings.TrimSpace(requested) == "" {
		return t.catalogs[t.defaultIndex].name
	}
	tags, _, err := language.ParseAcceptLanguage(requested)
	if err != nil || len(tags) == 0 {
		tags = []language.Tag{language.Make(requested)}
	}
	_, index, confidence := t.matcher.Match(tags...)
	if confidence == language.No {
		index = t.defaultIndex
	}
	return t.catalogs[index].name
}

// Trans 翻译消息键、带参数消息或兼容期内的英文消息；未知消息保持原样。
func (t *Translator) Trans(ctx context.Context, message string) string {
	translated, _ := t.translate(t.Language(ctx), message)
	return translated
}

func (t *Translator) translate(localeName, message string) (string, bool) {
	index := t.indexForName(localeName)
	id, args, encoded := decodeMessage(message)
	if encoded {
		for i, arg := range args {
			if nested, ok := t.translate(localeName, arg); ok {
				args[i] = nested
			}
		}
		if value, ok := t.catalogs[index].messages[id]; ok {
			return renderTemplate(value, args), true
		}
		if value, ok := t.catalogs[t.englishIndex()].messages[id]; ok {
			return renderTemplate(value, args), true
		}
		return id, false
	}

	if value, ok := t.catalogs[index].messages[message]; ok {
		return value, true
	}
	if id, ok := t.sourceToID[message]; ok {
		if value, exists := t.catalogs[index].messages[id]; exists {
			return value, true
		}
	}
	for _, candidate := range t.templates {
		matches := candidate.pattern.FindStringSubmatch(message)
		if matches == nil {
			continue
		}
		if value, ok := t.catalogs[index].messages[candidate.id]; ok {
			return renderTemplate(value, matches[1:]), true
		}
	}
	return message, false
}

func (t *Translator) indexForName(name string) int {
	for i := range t.catalogs {
		if t.catalogs[i].name == name {
			return i
		}
	}
	return t.defaultIndex
}

func (t *Translator) englishIndex() int {
	_, index, _ := t.matcher.Match(language.English)
	return index
}

func (t *Translator) buildSourceIndex() {
	t.buildSourceIndexFor(t.englishIndex())
}

func (t *Translator) buildSourceIndexFor(index int) {
	messages := t.catalogs[index].messages
	ids := make([]string, 0, len(messages))
	for id := range messages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		source := messages[id]
		if printfDirective.MatchString(source) {
			t.templates = append(t.templates, messageTemplate{id: id, pattern: compileTemplate(source)})
			continue
		}
		if source != "" {
			t.sourceToID[source] = id
		}
	}
}

func loadCatalogs(fsys fs.FS, root string) (map[string]map[string]string, error) {
	result := make(map[string]map[string]string)
	err := fs.WalkDir(fsys, root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".json" {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		flat := make(map[string]string)
		if err := flattenMessages("", raw, flat); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		result[name] = flat
		return nil
	})
	return result, err
}

func flattenMessages(prefix string, values map[string]any, result map[string]string) error {
	for key, value := range values {
		id := key
		if prefix != "" {
			id = prefix + "." + key
		}
		switch typed := value.(type) {
		case string:
			result[id] = typed
		case map[string]any:
			if err := flattenMessages(id, typed, result); err != nil {
				return err
			}
		default:
			return fmt.Errorf("message %s must be a string or object", id)
		}
	}
	return nil
}

func mergeCatalogs(base, overrides map[string]map[string]string) {
	for name, messages := range overrides {
		if base[name] == nil {
			base[name] = make(map[string]string)
		}
		for id, value := range messages {
			base[name][id] = value
		}
	}
}

func decodeMessage(message string) (string, []string, bool) {
	if !strings.HasPrefix(message, messagePrefix) {
		return message, nil, false
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(message, messagePrefix))
	if err != nil {
		return message, nil, false
	}
	var payload encodedMessage
	if err := json.Unmarshal(data, &payload); err != nil || payload.ID == "" {
		return message, nil, false
	}
	return payload.ID, payload.Args, true
}

func compileTemplate(value string) *regexp.Regexp {
	var pattern strings.Builder
	pattern.WriteString("(?s)^")
	last := 0
	for _, location := range printfDirective.FindAllStringIndex(value, -1) {
		pattern.WriteString(regexp.QuoteMeta(value[last:location[0]]))
		pattern.WriteString("(.*?)")
		last = location[1]
	}
	pattern.WriteString(regexp.QuoteMeta(value[last:]))
	pattern.WriteString("$")
	return regexp.MustCompile(pattern.String())
}

func renderTemplate(value string, args []string) string {
	locations := printfDirective.FindAllStringIndex(value, -1)
	if len(locations) == 0 || len(args) == 0 {
		return value
	}
	var result strings.Builder
	last := 0
	for i, location := range locations {
		result.WriteString(value[last:location[0]])
		if i < len(args) {
			result.WriteString(args[i])
		} else {
			result.WriteString(value[location[0]:location[1]])
		}
		last = location[1]
	}
	result.WriteString(value[last:])
	return result.String()
}
