package service

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/xlzd/gotp"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/netproxy"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
	"github.com/mhsanaei/3x-ui/v3/internal/util/reflect_util"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

//go:embed config.json
var xrayTemplateConfig string

var defaultValueMap = map[string]string{
	"xrayTemplateConfig":         xrayTemplateConfig,
	"xrayOutboundTestUrl":        "https://www.google.com/generate_204",
	"webListen":                  "",
	"webDomain":                  "",
	"webPort":                    "2053",
	"webCertFile":                "",
	"webKeyFile":                 "",
	"webBasePath":                normalizeWebBasePath(getEnv("XUI_INIT_WEB_BASE_PATH", "/")),
	"sessionMaxAge":              "360",
	"trustedProxyCIDRs":          "127.0.0.1/32,::1/128",
	"secret":                     random.Seq(32),
	"pageSize":                   "25",
	"expireDiff":                 "0",
	"trafficDiff":                "0",
	"datepicker":                 "gregorian",
	"timeLocation":               "Local",
	"twoFactorEnable":            "false",
	"twoFactorToken":             "",
	"panelOutbound":              "",
	"restartXrayOnClientDisable": "true",
	"warp":                       "",
	"warpLastUpdate":             "0",
	"warpUpdateInterval":         "0",
	"nord":                       "",
}

// SettingService owns the settings that are meaningful to the local panel.
type SettingService struct{}

func (s *SettingService) GetDefaultJSONConfig() (any, error) {
	return s.GetDefaultXrayConfig()
}

func (s *SettingService) GetDefaultXrayConfig() (any, error) {
	var value any
	if err := json.Unmarshal([]byte(xrayTemplateConfig), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *SettingService) GetAllSetting() (*entity.AllSetting, error) {
	var settings []*model.Setting
	if err := database.GetDB().
		Where("key <> ?", "xrayTemplateConfig").
		Find(&settings).Error; err != nil {
		return nil, err
	}

	result := &entity.AllSetting{}
	value := reflect.ValueOf(result).Elem()
	fields := reflect_util.GetFields(reflect.TypeFor[entity.AllSetting]())
	byKey := make(map[string]string, len(settings))
	for _, setting := range settings {
		byKey[setting.Key] = setting.Value
	}

	for _, field := range fields {
		key := field.Tag.Get("json")
		stored, ok := byKey[key]
		if !ok {
			stored = defaultValueMap[key]
		}
		if err := setSettingField(value.FieldByName(field.Name), key, stored); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func setSettingField(field reflect.Value, key, stored string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New(fmt.Sprint(recovered))
		}
	}()

	switch field.Kind() {
	case reflect.Int:
		parsed, parseErr := strconv.ParseInt(effectiveSettingValue(key, stored), 10, 64)
		if parseErr != nil {
			return parseErr
		}
		field.SetInt(parsed)
	case reflect.String:
		field.SetString(stored)
	case reflect.Bool:
		field.SetBool(effectiveSettingValue(key, stored) == "true")
	default:
		return common.NewErrorf("unknown setting field %s type %s", key, field.Kind())
	}
	return nil
}

func (s *SettingService) GetAllSettingView() (*entity.AllSettingView, error) {
	settings, err := s.GetAllSetting()
	if err != nil {
		return nil, err
	}
	view := &entity.AllSettingView{
		AllSetting:        *settings,
		HasTwoFactorToken: secretConfigured(settings.TwoFactorToken),
		HasWarpSecret:     secretConfigured(mustString(s.GetWarp())),
		HasNordSecret:     secretConfigured(mustString(s.GetNord())),
	}
	view.TwoFactorToken = ""
	return view, nil
}

func (s *SettingService) UpdateAllSetting(settings *entity.AllSetting) error {
	if settings.TwoFactorEnable && strings.TrimSpace(settings.TwoFactorToken) == "" {
		token, err := s.GetTwoFactorToken()
		if err != nil {
			return err
		}
		settings.TwoFactorToken = token
	}
	if !settings.TwoFactorEnable {
		settings.TwoFactorToken = ""
	}
	if err := settings.CheckValid(); err != nil {
		return err
	}

	value := reflect.ValueOf(settings).Elem()
	fields := reflect_util.GetFields(reflect.TypeFor[entity.AllSetting]())
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, field := range fields {
			key := field.Tag.Get("json")
			stored := fmt.Sprint(value.FieldByName(field.Name).Interface())
			var setting model.Setting
			result := tx.Where("key = ?", key).First(&setting)
			switch {
			case errors.Is(result.Error, gorm.ErrRecordNotFound):
				if err := tx.Create(&model.Setting{Key: key, Value: stored}).Error; err != nil {
					return err
				}
			case result.Error != nil:
				return result.Error
			case setting.Value != stored:
				if err := tx.Model(&setting).Update("value", stored).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *SettingService) ResetSettings() error {
	return database.GetDB().Where("1 = 1").Delete(&model.Setting{}).Error
}

func (s *SettingService) getSetting(key string) (*model.Setting, error) {
	setting := &model.Setting{}
	if err := database.GetDB().Where("key = ?", key).First(setting).Error; err != nil {
		return nil, err
	}
	return setting, nil
}

func (s *SettingService) saveSetting(key, value string) error {
	setting, err := s.getSetting(key)
	switch {
	case database.IsNotFound(err):
		return database.GetDB().Create(&model.Setting{Key: key, Value: value}).Error
	case err != nil:
		return err
	default:
		return database.GetDB().Model(setting).Update("value", value).Error
	}
}

func (s *SettingService) getString(key string) (string, error) {
	setting, err := s.getSetting(key)
	if database.IsNotFound(err) {
		value, ok := defaultValueMap[key]
		if !ok {
			return "", common.NewErrorf("key <%v> not in defaultValueMap", key)
		}
		return value, nil
	}
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (s *SettingService) setString(key, value string) error {
	return s.saveSetting(key, value)
}

func effectiveSettingValue(key, stored string) string {
	if stored == "" {
		if value, ok := defaultValueMap[key]; ok {
			return value
		}
	}
	return stored
}

func (s *SettingService) getBool(key string) (bool, error) {
	value, err := s.getString(key)
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(effectiveSettingValue(key, value))
}

func (s *SettingService) setBool(key string, value bool) error {
	return s.setString(key, strconv.FormatBool(value))
}

func (s *SettingService) getInt(key string) (int, error) {
	value, err := s.getString(key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(effectiveSettingValue(key, value))
}

func (s *SettingService) setInt(key string, value int) error {
	return s.setString(key, strconv.Itoa(value))
}

func (s *SettingService) GetXrayConfigTemplate() (string, error) {
	return s.getString("xrayTemplateConfig")
}

func (s *SettingService) GetXrayOutboundTestUrl() (string, error) {
	return s.getString("xrayOutboundTestUrl")
}

func (s *SettingService) SetXrayOutboundTestUrl(value string) error {
	clean, err := SanitizeHTTPURL(value)
	if err != nil {
		return err
	}
	return s.setString("xrayOutboundTestUrl", clean)
}

func (s *SettingService) GetListen() (string, error) {
	return s.getString("webListen")
}

func (s *SettingService) SetListen(value string) error {
	return s.setString("webListen", value)
}

func (s *SettingService) GetWebDomain() (string, error) {
	return s.getString("webDomain")
}

func (s *SettingService) GetPort() (int, error) {
	return s.getInt("webPort")
}

func (s *SettingService) SetPort(value int) error {
	return s.setInt("webPort", value)
}

func (s *SettingService) GetCertFile() (string, error) {
	return s.getString("webCertFile")
}

func (s *SettingService) SetCertFile(value string) error {
	return s.setString("webCertFile", value)
}

func (s *SettingService) GetKeyFile() (string, error) {
	return s.getString("webKeyFile")
}

func (s *SettingService) SetKeyFile(value string) error {
	return s.setString("webKeyFile", value)
}

func (s *SettingService) GetBasePath() (string, error) {
	value, err := s.getString("webBasePath")
	if err != nil {
		return "", err
	}
	return normalizeWebBasePath(value), nil
}

func (s *SettingService) SetBasePath(value string) error {
	return s.setString("webBasePath", normalizeWebBasePath(value))
}

func normalizeWebBasePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if !strings.HasSuffix(value, "/") {
		value += "/"
	}
	return value
}

func (s *SettingService) GetSessionMaxAge() (int, error) {
	return s.getInt("sessionMaxAge")
}

func (s *SettingService) GetTrustedProxyCIDRs() (string, error) {
	return s.getString("trustedProxyCIDRs")
}

func (s *SettingService) GetSecret() ([]byte, error) {
	secret, err := s.getString("secret")
	if err == nil {
		if _, findErr := s.getSetting("secret"); database.IsNotFound(findErr) {
			if saveErr := s.saveSetting("secret", secret); saveErr != nil {
				logger.Warning("save secret failed:", saveErr)
			}
		}
	}
	return []byte(secret), err
}

func (s *SettingService) GetTimeLocation() (*time.Location, error) {
	name, err := s.getString("timeLocation")
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(name)
	if err == nil {
		return location, nil
	}
	logger.Warningf("invalid time location %q, using UTC: %v", name, err)
	return time.UTC, nil
}

func (s *SettingService) GetExpireDiff() (int, error) {
	return s.getInt("expireDiff")
}

func (s *SettingService) GetTrafficDiff() (int, error) {
	return s.getInt("trafficDiff")
}

func (s *SettingService) GetPageSize() (int, error) {
	return s.getInt("pageSize")
}

func (s *SettingService) GetDatepicker() (string, error) {
	return s.getString("datepicker")
}

func (s *SettingService) GetTwoFactorEnable() (bool, error) {
	return s.getBool("twoFactorEnable")
}

func (s *SettingService) SetTwoFactorEnable(value bool) error {
	return s.setBool("twoFactorEnable", value)
}

func (s *SettingService) GetTwoFactorToken() (string, error) {
	return s.getString("twoFactorToken")
}

func (s *SettingService) SetTwoFactorToken(value string) error {
	return s.setString("twoFactorToken", strings.TrimSpace(value))
}

func (s *SettingService) VerifyTwoFactorCode(code string) error {
	enabled, err := s.GetTwoFactorEnable()
	if err != nil || !enabled {
		return err
	}
	token, err := s.GetTwoFactorToken()
	if err != nil {
		return err
	}
	if token == "" || !gotp.NewDefaultTOTP(token).Verify(strings.TrimSpace(code), time.Now().Unix()) {
		return common.NewError("invalid two factor code")
	}
	return nil
}

func (s *SettingService) GetPanelOutbound() (string, error) {
	return s.getString("panelOutbound")
}

func (s *SettingService) SetPanelOutbound(value string) error {
	return s.setString("panelOutbound", value)
}

func (s *SettingService) PanelEgressProxyURL() string {
	tag, err := s.GetPanelOutbound()
	if err != nil || tag == "" {
		return ""
	}
	process := XrayProcess()
	if process == nil || !process.IsRunning() || process.GetConfig() == nil {
		return ""
	}
	for _, inbound := range process.GetConfig().InboundConfigs {
		if inbound.Tag == PanelEgressInboundTag {
			return fmt.Sprintf("socks5://127.0.0.1:%d", inbound.Port)
		}
	}
	return ""
}

func (s *SettingService) NewProxiedHTTPClient(timeout time.Duration) *http.Client {
	client, err := netproxy.NewHTTPClient(s.PanelEgressProxyURL(), timeout)
	if err != nil {
		logger.Warningf("invalid panel egress proxy, using direct connection: %v", err)
		return &http.Client{Timeout: timeout}
	}
	return client
}

func (s *SettingService) GetRestartXrayOnClientDisable() (bool, error) {
	return s.getBool("restartXrayOnClientDisable")
}

func (s *SettingService) SetRestartXrayOnClientDisable(value bool) error {
	return s.setBool("restartXrayOnClientDisable", value)
}

func (s *SettingService) GetWarp() (string, error) {
	return s.getString("warp")
}

func (s *SettingService) SetWarp(value string) error {
	return s.setString("warp", value)
}

func (s *SettingService) GetNord() (string, error) {
	return s.getString("nord")
}

func (s *SettingService) SetNord(value string) error {
	return s.setString("nord", value)
}

func (s *SettingService) GetWarpLastUpdate() (int64, error) {
	value, err := s.getString("warpLastUpdate")
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(value, 10, 64)
}

func (s *SettingService) SetWarpLastUpdate(value int64) error {
	return s.setString("warpLastUpdate", strconv.FormatInt(value, 10))
}

func (s *SettingService) SetWarpUpdateInterval(value int) error {
	return s.setInt("warpUpdateInterval", value)
}

func (s *SettingService) GetAccessLogEnable() (bool, error) {
	path, err := xray.GetAccessLogPath()
	if err != nil {
		return false, err
	}
	return path != "" && path != "none", nil
}

func (s *SettingService) GetDefaultSettings(_ string) (map[string]any, error) {
	type getter func() (any, error)
	getters := map[string]getter{
		"expireDiff":      func() (any, error) { return s.GetExpireDiff() },
		"trafficDiff":     func() (any, error) { return s.GetTrafficDiff() },
		"pageSize":        func() (any, error) { return s.GetPageSize() },
		"defaultCert":     func() (any, error) { return s.GetCertFile() },
		"defaultKey":      func() (any, error) { return s.GetKeyFile() },
		"datepicker":      func() (any, error) { return s.GetDatepicker() },
		"accessLogEnable": func() (any, error) { return s.GetAccessLogEnable() },
		"webDomain":       func() (any, error) { return s.GetWebDomain() },
	}
	result := make(map[string]any, len(getters))
	for key, get := range getters {
		value, err := get()
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func secretConfigured(value string) bool {
	return strings.TrimSpace(value) != ""
}

func mustString(value string, _ error) string {
	return value
}

func getEnv(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
