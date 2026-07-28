package controller

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/acmecert"
	"github.com/mhsanaei/3x-ui/v3/internal/web/middleware"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type issueCertificateForm struct {
	Remark           string `json:"remark" form:"remark" validate:"required,max=120"`
	AddMethod        string `json:"addMethod" form:"addMethod" validate:"required,oneof=acme"`
	CA               string `json:"ca" form:"ca" validate:"required,oneof=zerossl letsencrypt"`
	ValidationMethod string `json:"validationMethod" form:"validationMethod" validate:"required,oneof=cloudflare"`
	CloudflareToken  string `json:"cloudflareToken" form:"cloudflareToken" validate:"required,max=4096"`
	Identifiers      string `json:"identifiers" form:"identifiers" validate:"required,max=16384"`
	Email            string `json:"email" form:"email" validate:"required,email,max=254"`
	KeyType          string `json:"keyType" form:"keyType" validate:"required,oneof=EC256 EC384 RSA2048 RSA4096"`
	CertificateType  string `json:"certificateType" form:"certificateType" validate:"required,oneof=domain ip"`
}

type certificateConfigForm struct {
	RenewBeforeDays       int    `json:"renewBeforeDays" form:"renewBeforeDays" validate:"gte=1,lte=90"`
	ShortRenewBeforeHours int    `json:"shortRenewBeforeHours" form:"shortRenewBeforeHours" validate:"gte=24,lte=168"`
	ShortCheckTimesPerDay int    `json:"shortCheckTimesPerDay" form:"shortCheckTimesPerDay" validate:"gte=4,lte=6"`
	CheckTime             string `json:"checkTime" form:"checkTime" validate:"required,max=8"`
	DefaultEmail          string `json:"defaultEmail" form:"defaultEmail" validate:"omitempty,email,max=254"`
	GlobalPrivateKey      string `json:"globalPrivateKey" form:"globalPrivateKey" validate:"max=16384"`
}

type CertificateController struct {
	certificateService service.CertificateService
}

func NewCertificateController(group *gin.RouterGroup) *CertificateController {
	controller := &CertificateController{}
	controller.initRouter(group)
	return controller
}

func (a *CertificateController) initRouter(group *gin.RouterGroup) {
	certificates := group.Group("/certificates")
	certificates.GET("/list", a.list)
	certificates.GET("/config", a.getConfig)
	certificates.POST("/config", a.updateConfig)
	certificates.POST("/issue", a.issue)
}

func (a *CertificateController) list(c *gin.Context) {
	certificates, err := a.certificateService.List()
	jsonObj(c, certificates, err)
}

func (a *CertificateController) getConfig(c *gin.Context) {
	config, err := a.certificateService.GetConfig()
	jsonObj(c, config, err)
}

func (a *CertificateController) updateConfig(c *gin.Context) {
	form, ok := middleware.BindAndValidate[certificateConfigForm](c)
	if !ok {
		return
	}
	config := &service.CertificateConfig{
		RenewBeforeDays:       form.RenewBeforeDays,
		ShortRenewBeforeHours: form.ShortRenewBeforeHours,
		ShortCheckTimesPerDay: form.ShortCheckTimesPerDay,
		CheckTime:             form.CheckTime,
		DefaultEmail:          form.DefaultEmail,
		GlobalPrivateKey:      form.GlobalPrivateKey,
	}
	if err := a.certificateService.SaveConfig(config); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.certificates.configSaveError"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.certificates.configSaveSuccess"), config, nil)
}

func (a *CertificateController) issue(c *gin.Context) {
	form, ok := middleware.BindAndValidate[issueCertificateForm](c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	result, err := a.certificateService.Issue(ctx, acmecert.IssueRequest{
		Remark:           form.Remark,
		AddMethod:        form.AddMethod,
		CA:               form.CA,
		ValidationMethod: form.ValidationMethod,
		CloudflareToken:  form.CloudflareToken,
		Identifiers:      form.Identifiers,
		Email:            form.Email,
		KeyType:          form.KeyType,
		CertificateType:  form.CertificateType,
	})
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.certificates.issueError"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.certificates.issueSuccess"), result, nil)
}
