package cloud

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const rechargeContactSettingKey = "recharge_contact_v1"

type rechargeContact struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func loadRechargeContact() (rechargeContact, error) {
	var raw []byte
	err := instanceDB.QueryRow(`SELECT setting_value FROM xcloud_settings WHERE setting_key=?`, rechargeContactSettingKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rechargeContact{}, nil
		}
		return rechargeContact{}, err
	}
	var value rechargeContact
	return value, json.Unmarshal(raw, &value)
}

func validRechargeContact(value rechargeContact) bool {
	if len([]rune(strings.TrimSpace(value.Name))) == 0 || len([]rune(value.Name)) > 64 {
		return false
	}
	u, err := url.ParseRequestURI(strings.TrimSpace(value.URL))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func rechargeContactHandler(c *gin.Context) {
	value, err := loadRechargeContact()
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(200, value)
}

func adminRechargeContactHandler(c *gin.Context) {
	if c.Request.Method == "GET" {
		rechargeContactHandler(c)
		return
	}
	var value rechargeContact
	if c.ShouldBindJSON(&value) != nil || !validRechargeContact(value) {
		c.JSON(400, gin.H{"message": "请填写有效的群名称和 http(s) 咨询地址"})
		return
	}
	value.Name, value.URL = strings.TrimSpace(value.Name), strings.TrimSpace(value.URL)
	raw, _ := json.Marshal(value)
	if _, err := instanceDB.Exec(`INSERT INTO xcloud_settings (setting_key,setting_value,updated_at) VALUES (?,CAST(? AS JSON),NOW()) ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value),updated_at=NOW()`, rechargeContactSettingKey, raw); err != nil {
		internalError(c, err)
		return
	}
	user := c.MustGet("user").(oidcUser)
	_ = writeAudit(c.Request.Context(), user.ID, "settings.recharge_contact.save", "setting", rechargeContactSettingKey, map[string]any{"name": value.Name})
	c.JSON(200, value)
}
