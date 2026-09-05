package cloud

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func quotePurchaseHandler(c *gin.Context) {
	var body struct {
		PlanID    string `json:"planId"`
		ImageID   string `json:"imageId"`
		Months    int    `json:"months"`
		PromoCode string `json:"promoCode"`
	}
	if c.ShouldBindJSON(&body) != nil || !validSubscriptionMonths(body.Months) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "请选择 1、3、6 或 12 个月"})
		return
	}
	var monthly int
	if err := instanceDB.QueryRowContext(c, `SELECT monthly_price_fen FROM xcloud_plans WHERE id=? AND enabled=TRUE`, body.PlanID).Scan(&monthly); err != nil {
		businessError(c, errors.New("套餐不可购买"))
		return
	}
	var exists int
	if err := instanceDB.QueryRowContext(c, `SELECT 1 FROM xcloud_images WHERE id=? AND enabled=TRUE`, body.ImageID).Scan(&exists); err != nil {
		businessError(c, errors.New("镜像版本不可购买"))
		return
	}
	tx, err := instanceDB.BeginTx(c, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	user := c.MustGet("user").(oidcUser)
	quote, err := quoteCommercialBenefit(c, user.ID, "purchase", body.PlanID, "", body.Months, monthly, body.PromoCode, tx, false)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, quote)
}

func quoteRenewHandler(c *gin.Context) {
	var body struct {
		Months    int    `json:"months"`
		PromoCode string `json:"promoCode"`
	}
	if c.ShouldBindJSON(&body) != nil || !validSubscriptionMonths(body.Months) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "请选择 1、3、6 或 12 个月"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	var planID, imageID, instanceID string
	var monthly int
	if err := instanceDB.QueryRowContext(c, `SELECT o.plan_id,o.image_id,COALESCE(o.instance_id,''),p.monthly_price_fen FROM xcloud_orders o JOIN xcloud_plans p ON p.id=o.plan_id WHERE o.id=? AND o.owner_id=?`, c.Param("id"), user.ID).Scan(&planID, &imageID, &instanceID, &monthly); err != nil {
		businessError(c, errors.New("订单不可续费"))
		return
	}
	tx, err := instanceDB.BeginTx(c, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()
	quote, err := quoteCommercialBenefit(c, user.ID, "renewal", planID, instanceID, body.Months, monthly, body.PromoCode, tx, false)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, quote)
}
