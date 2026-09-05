package cloud

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func createTicketHandler(c *gin.Context) {
	var body struct {
		Category   string `json:"category"`
		Priority   string `json:"priority"`
		Subject    string `json:"subject"`
		Body       string `json:"body"`
		InstanceID string `json:"instanceId"`
		OrderID    string `json:"orderId"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "工单参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, err := createTicket(c.Request.Context(), user.ID, strings.TrimSpace(body.Category), strings.TrimSpace(body.Priority), body.Subject, body.Body, body.InstanceID, body.OrderID)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func myTicketsHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	items, err := listUserTickets(c.Request.Context(), user.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func ticketDetailHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	item, err := ticketByID(c.Request.Context(), c.Param("id"), user.ID, false)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func ticketReplyHandler(c *gin.Context) {
	var body struct {
		Body string `json:"body"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "回复参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, err := replyTicket(c.Request.Context(), c.Param("id"), user.ID, "user", body.Body)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func reopenTicketHandler(c *gin.Context) {
	user := c.MustGet("user").(oidcUser)
	item, err := reopenTicket(c.Request.Context(), c.Param("id"), user.ID)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func adminTicketsHandler(c *gin.Context) {
	items, err := listAdminTickets(c.Request.Context(), strings.TrimSpace(c.Query("status")), strings.TrimSpace(c.Query("priority")))
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func adminTicketDetailHandler(c *gin.Context) {
	item, err := ticketByID(c.Request.Context(), c.Param("id"), "", true)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func adminTicketReplyHandler(c *gin.Context) {
	var body struct {
		Body string `json:"body"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "回复参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, err := replyTicket(c.Request.Context(), c.Param("id"), user.ID, "admin", body.Body)
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func adminTicketStatusHandler(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "状态参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, err := changeTicketStatus(c.Request.Context(), c.Param("id"), user.ID, strings.TrimSpace(body.Status))
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func adminTicketPriorityHandler(c *gin.Context) {
	var body struct {
		Priority string `json:"priority"`
	}
	if c.ShouldBindJSON(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "优先级参数无效"})
		return
	}
	user := c.MustGet("user").(oidcUser)
	item, err := changeTicketPriority(c.Request.Context(), c.Param("id"), user.ID, strings.TrimSpace(body.Priority))
	if err != nil {
		businessError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}
