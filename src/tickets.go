package cloud

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	ticketOpen       = "open"
	ticketInProgress = "in_progress"
	ticketClosed     = "closed"
)

type ticket struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"ownerId"`
	Category    string     `json:"category"`
	Priority    string     `json:"priority"`
	Subject     string     `json:"subject"`
	InstanceID  string     `json:"instanceId,omitempty"`
	OrderID     string     `json:"orderId,omitempty"`
	Status      string     `json:"status"`
	LastAdminID string     `json:"lastAdminId,omitempty"`
	ClosedAt    *time.Time `json:"closedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type ticketMessage struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticketId"`
	SenderID   string    `json:"senderId"`
	SenderRole string    `json:"senderRole"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ticketDetail struct {
	Ticket   ticket          `json:"ticket"`
	Messages []ticketMessage `json:"messages"`
}

func validTicketCategory(value string) bool {
	switch value {
	case "instance", "billing", "account", "other":
		return true
	}
	return false
}

func validTicketPriority(value string) bool {
	return value == "normal" || value == "high" || value == "urgent"
}

func validTicketStatus(value string) bool {
	return value == ticketOpen || value == ticketInProgress || value == ticketClosed
}

func normalizedTicketText(value string, maximum int, field string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New(field + "不能为空")
	}
	if len([]rune(value)) > maximum {
		return "", errors.New(field + "不能超过" + strconv.Itoa(maximum) + "个字符")
	}
	return value, nil
}

func validateTicketReference(ctx context.Context, ownerID, instanceID, orderID string) error {
	if instanceID != "" {
		var found int
		if err := instanceDB.QueryRowContext(ctx, `SELECT 1 FROM xcloud_instances WHERE id=? AND owner_id=?`, instanceID, ownerID).Scan(&found); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("关联实例不存在或无权访问")
			}
			return err
		}
	}
	if orderID != "" {
		var found int
		if err := instanceDB.QueryRowContext(ctx, `SELECT 1 FROM xcloud_orders WHERE id=? AND owner_id=?`, orderID, ownerID).Scan(&found); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("关联订单不存在或无权访问")
			}
			return err
		}
	}
	return nil
}

func createTicket(ctx context.Context, ownerID, category, priority, subject, body, instanceID, orderID string) (ticket, error) {
	if !validTicketCategory(category) || !validTicketPriority(priority) {
		return ticket{}, errors.New("工单分类或优先级无效")
	}
	var err error
	if subject, err = normalizedTicketText(subject, 160, "工单主题"); err != nil {
		return ticket{}, err
	}
	if body, err = normalizedTicketText(body, 4000, "工单内容"); err != nil {
		return ticket{}, err
	}
	instanceID, orderID = strings.TrimSpace(instanceID), strings.TrimSpace(orderID)
	if err := validateTicketReference(ctx, ownerID, instanceID, orderID); err != nil {
		return ticket{}, err
	}
	now := time.Now()
	item := ticket{ID: newID("tkt"), OwnerID: ownerID, Category: category, Priority: priority, Subject: subject, InstanceID: instanceID, OrderID: orderID, Status: ticketOpen, CreatedAt: now, UpdatedAt: now}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return ticket{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_tickets (id,owner_id,category,priority,subject,instance_id,order_id,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, item.ID, ownerID, category, priority, subject, nullableString(instanceID), nullableString(orderID), ticketOpen, now, now); err != nil {
		return ticket{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_ticket_messages (id,ticket_id,sender_id,sender_role,body,created_at) VALUES (?,?,?,?,?,?)`, newID("tmsg"), item.ID, ownerID, "user", body, now); err != nil {
		return ticket{}, err
	}
	if err = writeAuditTx(ctx, tx, ownerID, "ticket.create", "ticket", item.ID, map[string]any{"category": category, "priority": priority}); err != nil {
		return ticket{}, err
	}
	if err = tx.Commit(); err != nil {
		return ticket{}, err
	}
	_ = createNotification(ctx, ownerID, "ticket", "工单已提交", "我们已收到你的工单，会尽快处理。", map[string]any{"ticketId": item.ID})
	return item, nil
}

func scanTickets(ctx context.Context, query string, args ...any) ([]ticket, error) {
	rows, err := instanceDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ticket{}
	for rows.Next() {
		var item ticket
		if err := rows.Scan(&item.ID, &item.OwnerID, &item.Category, &item.Priority, &item.Subject, &item.InstanceID, &item.OrderID, &item.Status, &item.LastAdminID, &item.ClosedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const ticketColumns = `id,owner_id,category,priority,subject,COALESCE(instance_id,''),COALESCE(order_id,''),status,COALESCE(last_admin_id,''),closed_at,created_at,updated_at`

func listUserTickets(ctx context.Context, ownerID string) ([]ticket, error) {
	return scanTickets(ctx, `SELECT `+ticketColumns+` FROM xcloud_tickets WHERE owner_id=? ORDER BY updated_at DESC`, ownerID)
}

func listAdminTickets(ctx context.Context, status, priority string) ([]ticket, error) {
	query, args := `SELECT `+ticketColumns+` FROM xcloud_tickets WHERE 1=1`, []any{}
	if status != "" {
		if !validTicketStatus(status) {
			return nil, errors.New("工单状态无效")
		}
		query, args = query+` AND status=?`, append(args, status)
	}
	if priority != "" {
		if !validTicketPriority(priority) {
			return nil, errors.New("工单优先级无效")
		}
		query, args = query+` AND priority=?`, append(args, priority)
	}
	query += ` ORDER BY CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 ELSE 2 END, updated_at DESC`
	return scanTickets(ctx, query, args...)
}

func ticketByID(ctx context.Context, id, ownerID string, admin bool) (ticketDetail, error) {
	query, args := `SELECT `+ticketColumns+` FROM xcloud_tickets WHERE id=?`, []any{id}
	if !admin {
		query, args = query+` AND owner_id=?`, append(args, ownerID)
	}
	var item ticket
	err := instanceDB.QueryRowContext(ctx, query, args...).Scan(&item.ID, &item.OwnerID, &item.Category, &item.Priority, &item.Subject, &item.InstanceID, &item.OrderID, &item.Status, &item.LastAdminID, &item.ClosedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ticketDetail{}, errors.New("工单不存在或无权访问")
	}
	if err != nil {
		return ticketDetail{}, err
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,ticket_id,sender_id,sender_role,body,created_at FROM xcloud_ticket_messages WHERE ticket_id=? ORDER BY created_at,id`, id)
	if err != nil {
		return ticketDetail{}, err
	}
	defer rows.Close()
	result := ticketDetail{Ticket: item, Messages: []ticketMessage{}}
	for rows.Next() {
		var message ticketMessage
		if err := rows.Scan(&message.ID, &message.TicketID, &message.SenderID, &message.SenderRole, &message.Body, &message.CreatedAt); err != nil {
			return ticketDetail{}, err
		}
		result.Messages = append(result.Messages, message)
	}
	return result, rows.Err()
}

func replyTicket(ctx context.Context, id, actorID, role, body string) (ticketDetail, error) {
	var err error
	if body, err = normalizedTicketText(body, 4000, "回复内容"); err != nil {
		return ticketDetail{}, err
	}
	tx, err := beginSerializableTx(ctx)
	if err != nil {
		return ticketDetail{}, err
	}
	defer tx.Rollback()
	var ownerID, status string
	if err = tx.QueryRowContext(ctx, `SELECT owner_id,status FROM xcloud_tickets WHERE id=? FOR UPDATE`, id).Scan(&ownerID, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ticketDetail{}, errors.New("工单不存在")
		}
		return ticketDetail{}, err
	}
	if role == "user" && ownerID != actorID {
		return ticketDetail{}, errors.New("工单不存在或无权访问")
	}
	if status == ticketClosed {
		return ticketDetail{}, errors.New("工单已关闭，请先重新打开")
	}
	now := time.Now()
	if _, err = tx.ExecContext(ctx, `INSERT INTO xcloud_ticket_messages (id,ticket_id,sender_id,sender_role,body,created_at) VALUES (?,?,?,?,?,?)`, newID("tmsg"), id, actorID, role, body, now); err != nil {
		return ticketDetail{}, err
	}
	if role == "admin" {
		_, err = tx.ExecContext(ctx, `UPDATE xcloud_tickets SET status=CASE WHEN status='open' THEN 'in_progress' ELSE status END,last_admin_id=?,updated_at=? WHERE id=?`, actorID, now, id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE xcloud_tickets SET updated_at=? WHERE id=?`, now, id)
	}
	if err != nil {
		return ticketDetail{}, err
	}
	if err = writeAuditTx(ctx, tx, actorID, "ticket.reply", "ticket", id, map[string]any{"role": role}); err != nil {
		return ticketDetail{}, err
	}
	if err = tx.Commit(); err != nil {
		return ticketDetail{}, err
	}
	if role == "admin" {
		_ = createNotification(ctx, ownerID, "ticket", "工单有新回复", "管理员已回复你的工单。", map[string]any{"ticketId": id})
	}
	return ticketByID(ctx, id, actorID, role == "admin")
}

func changeTicketStatus(ctx context.Context, id, actorID, status string) (ticketDetail, error) {
	if status != ticketInProgress && status != ticketClosed {
		return ticketDetail{}, errors.New("仅支持标记处理中或关闭工单")
	}
	now := time.Now()
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tickets SET status=?,last_admin_id=?,closed_at=CASE WHEN ?='closed' THEN ? ELSE NULL END,updated_at=? WHERE id=? AND status<>?`, status, actorID, status, now, now, id, ticketClosed)
	if err != nil {
		return ticketDetail{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ticketDetail{}, errors.New("工单不存在或当前不可变更")
	}
	detail, err := ticketByID(ctx, id, actorID, true)
	if err != nil {
		return ticketDetail{}, err
	}
	_ = writeAudit(ctx, actorID, "ticket.status", "ticket", id, map[string]any{"status": status})
	title := "工单处理中"
	body := "管理员正在处理你的工单。"
	if status == ticketClosed {
		title, body = "工单已关闭", "管理员已完成处理；如仍有问题可以重新打开工单。"
	}
	_ = createNotification(ctx, detail.Ticket.OwnerID, "ticket", title, body, map[string]any{"ticketId": id})
	return detail, nil
}

func changeTicketPriority(ctx context.Context, id, actorID, priority string) (ticketDetail, error) {
	if !validTicketPriority(priority) {
		return ticketDetail{}, errors.New("工单优先级无效")
	}
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tickets SET priority=?,last_admin_id=?,updated_at=NOW() WHERE id=?`, priority, actorID, id)
	if err != nil {
		return ticketDetail{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ticketDetail{}, errors.New("工单不存在")
	}
	_ = writeAudit(ctx, actorID, "ticket.priority", "ticket", id, map[string]any{"priority": priority})
	return ticketByID(ctx, id, actorID, true)
}

func reopenTicket(ctx context.Context, id, ownerID string) (ticketDetail, error) {
	result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tickets SET status=?,closed_at=NULL,updated_at=NOW() WHERE id=? AND owner_id=? AND status=?`, ticketOpen, id, ownerID, ticketClosed)
	if err != nil {
		return ticketDetail{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ticketDetail{}, errors.New("工单当前不可重新打开")
	}
	_ = writeAudit(ctx, ownerID, "ticket.reopen", "ticket", id, nil)
	_ = createNotification(ctx, ownerID, "ticket", "工单已重新打开", "你可以继续补充问题，管理员会再次处理。", map[string]any{"ticketId": id})
	return ticketByID(ctx, id, ownerID, false)
}
