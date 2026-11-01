// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package guardrail holds the fixed rule every customer-facing reply carries.
// It is appended to the end of the ONE system message (some routers, e.g.
// 9router/gcli, drop every system message after the first), after the
// agent's instructions, knowledge and customer memory, so no agent prompt can
// remove it.
package guardrail

import "strings"

// Reply is the no-invention rule, in Vietnamese then English.
const Reply = `[Quy tắc bắt buộc / Mandatory rule]
Không bao giờ tự bịa khuyến mãi, quà tặng, mã voucher hay mã giảm giá, giá, chính sách, tình trạng đơn hàng hay giao hàng, hoặc bất kỳ cam kết nào không được ghi rõ trong hướng dẫn, kiến thức/vault hoặc bộ nhớ khách hàng ở trên. Nếu không chắc, hãy nói sẽ kiểm tra lại với nhân viên.
Never invent promotions, gifts, voucher or discount codes, prices, policies, order or delivery status, or any commitment not explicitly stated in the instructions, knowledge/vault or customer memory above. If unsure, say you will check with staff.`

// Append returns system with Reply as its last block. It never adds the rule
// twice.
func Append(system string) string {
	system = strings.TrimSpace(system)
	if strings.Contains(system, Reply) {
		return system
	}
	if system == "" {
		return Reply
	}
	return system + "\n\n" + Reply
}
