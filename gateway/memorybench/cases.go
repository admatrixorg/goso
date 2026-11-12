// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memorybench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Group ids follow the Memory v3 bench table (Trích xuất, Suy luận, Cập nhật,
// Thẩm quyền, Từ chối, Cô lập, Tiếng Việt). This file is the 30-case sample,
// not the later 200-case set.
const (
	GroupExtract   = "trich-xuat"
	GroupMulti     = "suy-luan"
	GroupUpdate    = "cap-nhat"
	GroupAuthority = "tham-quyen"
	GroupRefuse    = "tu-choi"
	GroupIsolate   = "co-lap"
	GroupViet      = "tieng-viet"

	ZeroNeverSaid = "never-said"
	ZeroTrivial   = "trivial"
	ZeroIsolated  = "isolated"
)

// Group is one scored slice of the bench.
type Group struct {
	ID   string
	Name string
}

// Groups is the report order.
var Groups = []Group{
	{ID: GroupExtract, Name: "Trích xuất thông tin"},
	{ID: GroupMulti, Name: "Suy luận nhiều phiên"},
	{ID: GroupUpdate, Name: "Cập nhật tri thức"},
	{ID: GroupAuthority, Name: "Thẩm quyền"},
	{ID: GroupRefuse, Name: "Từ chối"},
	{ID: GroupIsolate, Name: "Cô lập"},
	{ID: GroupViet, Name: "Tiếng Việt không dấu, đồng nghĩa"},
}

// Note is one L1 row written before the question.
type Note struct {
	Session string `json:"session"`
	Agent   string `json:"agent,omitempty"`
	Tenant  string `json:"tenant,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Label   string `json:"label,omitempty"`
	Body    string `json:"body"`
}

// Case is one question against notes written in other sessions.
type Case struct {
	ID             string   `json:"id"`
	Group          string   `json:"group"`
	Title          string   `json:"title"`
	Tenant         string   `json:"tenant"`
	Notes          []Note   `json:"notes"`
	AskSession     string   `json:"ask_session"`
	AskAgent       string   `json:"ask_agent,omitempty"`
	AskTenant      string   `json:"ask_tenant,omitempty"`
	AskLabel       string   `json:"ask_label,omitempty"`
	CustomerScoped bool     `json:"customer_scoped,omitempty"`
	Query          string   `json:"query"`
	ExpectZero     bool     `json:"expect_zero,omitempty"`
	ZeroReason     string   `json:"zero_reason,omitempty"`
	MustContain    []string `json:"must_contain,omitempty"`
	MustNotContain []string `json:"must_not_contain,omitempty"`
}

// Cases is the Minh An sample. Every body is fictional demo copy.
func Cases() []Case {
	return []Case{
		{
			ID: "tx-01", Group: GroupExtract, Tenant: "minh-an",
			Title:      "Hỏi lại dạng báo giá đã nói ở phiên trước",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Công ty mẫu Minh An gửi báo giá dạng PDF có VAT."},
			},
			Query:       "Minh An gửi báo giá dạng gì",
			MustContain: []string{"PDF"},
		},
		{
			ID: "tx-02", Group: GroupExtract, Tenant: "minh-an",
			Title:      "Hỏi lại mức chiết khấu đã chốt",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindDurable, Body: "Chiết khấu nội bộ Minh An đã chốt tám phần trăm."},
			},
			Query:       "chiết khấu nội bộ Minh An đã chốt bao nhiêu",
			MustContain: []string{"tám phần trăm"},
		},
		{
			ID: "tx-03", Group: GroupExtract, Tenant: "minh-an",
			Title:      "Hỏi lại chất liệu của mã ghế mẫu",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Ghế mẫu Minh An mã MA-SOFA-01 làm bằng gỗ walnut."},
			},
			Query:       "mã MA-SOFA-01 làm bằng gì",
			MustContain: []string{"walnut"},
		},
		{
			ID: "tx-04", Group: GroupExtract, Tenant: "minh-an",
			Title:      "Hỏi lại vị trí kho mẫu",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Kho mẫu Minh An đặt tại khu A và không giao trước chín giờ."},
			},
			Query:       "kho mẫu Minh An đặt tại đâu",
			MustContain: []string{"khu A"},
		},
		{
			ID: "tx-05", Group: GroupExtract, Tenant: "minh-an",
			Title:      "Hỏi lại thời hạn bảo hành pháp chế đã ghi",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindDurable, Body: "Pháp chế Minh An yêu cầu bảo hành mẫu mười hai tháng."},
			},
			Query:       "pháp chế Minh An yêu cầu bảo hành bao lâu",
			MustContain: []string{"mười hai tháng"},
		},
		{
			ID: "tx-06", Group: GroupExtract, Tenant: "minh-an",
			Title:      "Phiên hiện tại không che ghi chú phiên trước",
			AskSession: "now",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Minh An gửi báo giá dạng PDF cho đơn mẫu."},
				{Session: "now", Kind: store.KindEpisodic, Body: "Minh An gửi báo giá dạng Excel trong phiên này."},
			},
			Query:          "Minh An gửi báo giá dạng gì",
			MustContain:    []string{"PDF"},
			MustNotContain: []string{"Excel"},
		},
		{
			ID: "sl-01", Group: GroupMulti, Tenant: "minh-an",
			Title:      "Ghép đơn mẫu với thời hạn bảo hành",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Đơn mẫu MA-SOFA-01 của Minh An gồm ghế walnut."},
				{Session: "s2", Kind: store.KindEpisodic, Body: "Ghế walnut thuộc đơn mẫu Minh An được bảo hành mười hai tháng."},
			},
			Query:       "Đơn mẫu MA-SOFA-01 bảo hành bao lâu",
			MustContain: []string{"MA-SOFA-01", "mười hai tháng"},
		},
		{
			ID: "sl-02", Group: GroupMulti, Tenant: "minh-an",
			Title:      "Ghép lịch nhận báo giá và điều kiện VAT",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Khách mẫu của Minh An thích nhận báo giá vào thứ hai."},
				{Session: "s2", Kind: store.KindEpisodic, Body: "Khách mẫu của Minh An thích nhận báo giá kèm VAT."},
			},
			Query:       "khách mẫu của Minh An thích nhận báo giá thế nào",
			MustContain: []string{"thứ hai", "VAT"},
		},
		{
			ID: "sl-03", Group: GroupMulti, Tenant: "minh-an",
			Title:      "Ghép giai đoạn, nhu cầu và nơi giao",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Dự án mẫu Minh An đang ở giai đoạn khảo sát."},
				{Session: "s2", Kind: store.KindEpisodic, Body: "Dự án mẫu Minh An cần ghế walnut."},
				{Session: "s3", Kind: store.KindEpisodic, Body: "Dự án mẫu Minh An giao tại kho khu A."},
			},
			Query:       "dự án mẫu Minh An cần gì và giao ở đâu",
			MustContain: []string{"walnut", "khu A"},
		},
		{
			ID: "sl-04", Group: GroupMulti, Tenant: "minh-an",
			Title:      "Ghép cách ký và nơi lưu hợp đồng mẫu",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Hợp đồng mẫu Minh An ký bằng chữ ký số."},
				{Session: "s2", Kind: store.KindEpisodic, Body: "Hợp đồng mẫu Minh An được lưu ở tủ pháp chế."},
			},
			Query:       "hợp đồng mẫu Minh An ký bằng gì và lưu ở đâu",
			MustContain: []string{"chữ ký số", "tủ pháp chế"},
		},
		{
			ID: "cn-01", Group: GroupUpdate, Tenant: "minh-an",
			Title:      "Đổi dạng báo giá từ PDF sang Excel",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Minh An thích báo giá dạng PDF."},
				{Session: "s2", Kind: store.KindEpisodic, Body: "Minh An thích báo giá dạng Excel kể từ hôm nay."},
			},
			Query:          "Minh An thích báo giá dạng gì",
			MustContain:    []string{"Excel"},
			MustNotContain: []string{"PDF"},
		},
		{
			ID: "cn-02", Group: GroupUpdate, Tenant: "minh-an",
			Title:      "Đổi chiết khấu từ tám phần trăm sang chín",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindDurable, Body: "Chiết khấu mẫu Minh An là tám phần trăm."},
				{Session: "s2", Kind: store.KindDurable, Body: "Chiết khấu mẫu Minh An là chín phần trăm kể từ tháng này."},
			},
			Query:          "chiết khấu mẫu Minh An hiện là bao nhiêu",
			MustContain:    []string{"chín phần trăm"},
			MustNotContain: []string{"tám phần trăm"},
		},
		{
			ID: "cn-03", Group: GroupUpdate, Tenant: "minh-an",
			Title:      "Đổi giai đoạn từ khảo sát sang thương lượng",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Dự án mẫu Minh An đang ở giai đoạn khảo sát."},
				{Session: "s2", Kind: store.KindEpisodic, Body: "Dự án mẫu Minh An đang ở giai đoạn thương lượng."},
			},
			Query:          "dự án mẫu Minh An đang ở giai đoạn nào",
			MustContain:    []string{"thương lượng"},
			MustNotContain: []string{"khảo sát"},
		},
		{
			ID: "cn-04", Group: GroupUpdate, Tenant: "minh-an",
			Title:      "Chuyển bộ phận sang pháp chế, có ngày hiệu lực",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Bộ phận mẫu Minh An trực thuộc phòng kinh doanh."},
				{Session: "s2", Kind: store.KindEpisodic, Body: "Bộ phận mẫu Minh An trực thuộc phòng pháp chế từ ngày mười một tháng mười một."},
			},
			Query:          "bộ phận mẫu Minh An trực thuộc phòng nào",
			MustContain:    []string{"pháp chế"},
			MustNotContain: []string{"kinh doanh"},
		},
		{
			ID: "tq-01", Group: GroupAuthority, Tenant: "minh-an",
			Title:      "Xác nhận chiết khấu của người dùng, extractor nói khác",
			AskSession: "ask",
			Notes: []Note{
				{Session: "user", Kind: store.KindDurable, Body: "Người dùng xác nhận chiết khấu mẫu Minh An là tám phần trăm."},
				{Session: "extractor", Kind: store.KindEpisodic, Body: "Extractor suy đoán chiết khấu mẫu Minh An là mười lăm phần trăm."},
			},
			Query:          "chiết khấu mẫu Minh An là bao nhiêu",
			MustContain:    []string{"tám phần trăm"},
			MustNotContain: []string{"mười lăm"},
		},
		{
			ID: "tq-02", Group: GroupAuthority, Tenant: "minh-an",
			Title:      "Xác nhận dạng báo giá, extractor nói khác",
			AskSession: "ask",
			Notes: []Note{
				{Session: "user", Kind: store.KindDurable, Body: "Người dùng xác nhận Minh An chỉ gửi báo giá dạng PDF."},
				{Session: "extractor", Kind: store.KindEpisodic, Body: "Extractor suy đoán Minh An gửi báo giá dạng Excel."},
			},
			Query:          "Minh An gửi báo giá dạng nào",
			MustContain:    []string{"PDF"},
			MustNotContain: []string{"Excel"},
		},
		{
			ID: "tq-03", Group: GroupAuthority, Tenant: "minh-an",
			Title:      "Xác nhận giờ gọi, extractor nói khác",
			AskSession: "ask",
			Notes: []Note{
				{Session: "user", Kind: store.KindDurable, Body: "Người dùng xác nhận giờ gọi mẫu của Minh An là sau chín giờ."},
				{Session: "extractor", Kind: store.KindEpisodic, Body: "Extractor suy đoán giờ gọi mẫu của Minh An là trước bảy giờ."},
			},
			Query:          "giờ gọi mẫu của Minh An là khi nào",
			MustContain:    []string{"sau chín giờ"},
			MustNotContain: []string{"trước bảy giờ"},
		},
		{
			ID: "tq-04", Group: GroupAuthority, Tenant: "minh-an",
			Title:      "Xác nhận bảo hành, extractor nói khác",
			AskSession: "ask",
			Notes: []Note{
				{Session: "user", Kind: store.KindDurable, Body: "Người dùng xác nhận bảo hành mẫu Minh An là mười hai tháng."},
				{Session: "extractor", Kind: store.KindEpisodic, Body: "Extractor suy đoán bảo hành mẫu Minh An là sáu tháng."},
			},
			Query:          "bảo hành mẫu Minh An là bao lâu",
			MustContain:    []string{"mười hai tháng"},
			MustNotContain: []string{"sáu tháng"},
		},
		{
			ID: "tc-01", Group: GroupRefuse, Tenant: "minh-an",
			Title:      "Hỏi chính sách nghỉ phép khi chưa từng nói",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Công ty mẫu giữ ghế sofa walnut mã nội bộ và gửi bản pdf có thuế giá trị gia tăng."},
			},
			Query:      "chính sách nghỉ phép ngày lễ tết",
			ExpectZero: true, ZeroReason: ZeroNeverSaid,
		},
		{
			ID: "tc-02", Group: GroupRefuse, Tenant: "minh-an",
			Title:      "Câu chào và cảm ơn không được nạp",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Minh An ghi nhận lời cảm ơn của khách mẫu sau buổi demo ghế."},
			},
			Query:      "ok cảm ơn",
			ExpectZero: true, ZeroReason: ZeroTrivial,
		},
		{
			ID: "tc-03", Group: GroupRefuse, Tenant: "minh-an",
			Title:      "Hỏi lịch trực đêm khi chưa từng nói",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Báo giá mẫu Minh An gồm ghế sofa walnut."},
			},
			Query:      "lịch trực đêm khu vực vệ sinh",
			ExpectZero: true, ZeroReason: ZeroNeverSaid,
		},
		{
			ID: "tc-04", Group: GroupRefuse, Tenant: "minh-an",
			Title:      "Hỏi máy lạnh và hộp mực khi chưa từng nói",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindDurable, Body: "Báo giá mẫu Minh An gồm ghế sofa walnut."},
			},
			Query:      "may lanh cong nghiep hop muc",
			ExpectZero: true, ZeroReason: ZeroNeverSaid,
		},
		{
			ID: "cl-01", Group: GroupIsolate, Tenant: "minh-an",
			Title:          "Khách mẫu B không thấy ghi chú khách mẫu A",
			AskSession:     "ask-b",
			AskLabel:       "zalo-personal:mau-b",
			CustomerScoped: true,
			Notes: []Note{
				{Session: "s-a", Label: "zalo-personal:mau-a", Kind: store.KindDurable, Body: "Khách mẫu kênh A của Minh An đặt ghế walnut mã MA-SOFA-01."},
			},
			Query:      "khách mẫu kênh đặt ghế walnut mã nào",
			ExpectZero: true, ZeroReason: ZeroIsolated,
		},
		{
			ID: "cl-02", Group: GroupIsolate, Tenant: "minh-an-sales",
			Title:      "Tenant pháp chế không thấy ghi chú tenant kinh doanh",
			AskSession: "ask",
			AskTenant:  "minh-an-legal",
			Notes: []Note{
				{Session: "s1", Tenant: "minh-an-sales", Kind: store.KindDurable, Body: "Chiết khấu mẫu phòng kinh doanh Minh An là chín phần trăm."},
			},
			Query:      "chiết khấu mẫu phòng kinh doanh Minh An là bao nhiêu",
			ExpectZero: true, ZeroReason: ZeroIsolated,
		},
		{
			ID: "cl-03", Group: GroupIsolate, Tenant: "minh-an",
			Title:      "Agent pháp chế không thấy ghi chú agent kinh doanh",
			AskSession: "ask",
			AskAgent:   "legal",
			Notes: []Note{
				{Session: "s1", Agent: "sales", Kind: store.KindEpisodic, Body: "Phòng kinh doanh Minh An giữ mẫu ghế walnut."},
			},
			Query:      "phòng kinh doanh Minh An giữ mẫu gì",
			ExpectZero: true, ZeroReason: ZeroIsolated,
		},
		{
			ID: "cl-04", Group: GroupIsolate, Tenant: "minh-an",
			Title:          "Khách mẫu A thấy ghi chú của mình, không thấy khách B",
			AskSession:     "ask-a",
			AskLabel:       "zalo-personal:mau-a",
			CustomerScoped: true,
			Notes: []Note{
				{Session: "s-a", Label: "zalo-personal:mau-a", Kind: store.KindDurable, Body: "Kênh mẫu A của Minh An chốt bảo hành mười hai tháng."},
				{Session: "s-b", Label: "zalo-personal:mau-b", Kind: store.KindDurable, Body: "Kênh mẫu B của Minh An chốt bảo hành sáu tháng."},
			},
			Query:          "kênh mẫu chốt bảo hành bao lâu",
			MustContain:    []string{"mười hai tháng"},
			MustNotContain: []string{"sáu tháng"},
		},
		{
			ID: "tv-01", Group: GroupViet, Tenant: "minh-an",
			Title:      "Có dấu: tìm báo giá",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Minh An gửi báo giá PDF cho đơn mẫu nội bộ."},
			},
			Query:       "báo giá PDF",
			MustContain: []string{"báo giá"},
		},
		{
			ID: "tv-02", Group: GroupViet, Tenant: "minh-an",
			Title:      "Có dấu: tìm bảo hành",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindDurable, Body: "Pháp chế Minh An yêu cầu bảo hành mười hai tháng cho đơn mẫu."},
			},
			Query:       "bảo hành mười hai tháng",
			MustContain: []string{"mười hai tháng"},
		},
		{
			ID: "tv-03", Group: GroupViet, Tenant: "minh-an",
			Title:      "Không dấu: gui bao gia phải gặp báo giá",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Minh An gửi báo giá PDF cho đơn mẫu nội bộ."},
			},
			Query:       "gui bao gia",
			MustContain: []string{"báo giá"},
		},
		{
			ID: "tv-04", Group: GroupViet, Tenant: "minh-an",
			Title:      "Đồng nghĩa: quotation phải gặp báo giá",
			AskSession: "ask",
			Notes: []Note{
				{Session: "s1", Kind: store.KindEpisodic, Body: "Minh An gửi báo giá PDF cho đơn mẫu nội bộ."},
			},
			Query:       "quotation sheet",
			MustContain: []string{"báo giá"},
		},
	}
}

var (
	emailRE = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRE = regexp.MustCompile(`\d{8,}`)
	banned  = []string{"mq capital", "mqcapital", "simildiet", "dewee", "goclaw", "hung-ai"}
)

func casesJSON() ([]byte, error) {
	raw, err := json.MarshalIndent(Cases(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func writeCasesJSON(path string) error {
	raw, err := casesJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func casesFileMatches() error {
	path := filepath.Join(BenchDir(), "cases.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	want, err := casesJSON()
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, want) {
		return fmt.Errorf("cases.json does not match Cases()")
	}
	return nil
}

// Validate checks the suite shape and the Minh An sample rule.
func Validate(cases []Case) error {
	if len(cases) != 30 {
		return fmt.Errorf("want 30 cases, got %d", len(cases))
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	neverSaid := 0
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] {
			return fmt.Errorf("bad id %q", c.ID)
		}
		seen[c.ID] = true
		if groupName(c.Group) == "" {
			return fmt.Errorf("%s: unknown group %s", c.ID, c.Group)
		}
		counts[c.Group]++
		if strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.Tenant) == "" || strings.TrimSpace(c.Query) == "" || c.AskSession == "" {
			return fmt.Errorf("%s: title, tenant, query, and ask session are required", c.ID)
		}
		if len(c.Notes) == 0 {
			return fmt.Errorf("%s: needs at least one note", c.ID)
		}
		askSeen := false
		for _, n := range c.Notes {
			if strings.TrimSpace(n.Session) == "" || strings.TrimSpace(n.Body) == "" {
				return fmt.Errorf("%s: note session and body are required", c.ID)
			}
			kind := n.Kind
			if kind == "" {
				kind = store.KindEpisodic
			}
			if kind != store.KindEpisodic && kind != store.KindDurable {
				return fmt.Errorf("%s: kind %s", c.ID, kind)
			}
			if n.Session == c.AskSession {
				askSeen = true
			}
		}
		if !askSeen && c.AskSession == "" {
			return fmt.Errorf("%s: ask session", c.ID)
		}
		if c.ExpectZero {
			if len(c.MustContain) != 0 || len(c.MustNotContain) != 0 {
				return fmt.Errorf("%s: a zero case has no gold text", c.ID)
			}
			switch c.ZeroReason {
			case ZeroTrivial:
				if !pipeline.IsTrivialQuery(c.Query) {
					return fmt.Errorf("%s: marked trivial but the query is not", c.ID)
				}
			case ZeroNeverSaid:
				neverSaid++
				if pipeline.IsTrivialQuery(c.Query) {
					return fmt.Errorf("%s: never-said query is trivial, so it does not test a real question", c.ID)
				}
				if shared := sharedTokens(c); len(shared) > 0 {
					return fmt.Errorf("%s: never-said query shares tokens %v with its notes", c.ID, shared)
				}
				if shared := sharedFoldedTokens(c); len(shared) > 0 {
					return fmt.Errorf("%s: never-said query folds onto note tokens %v", c.ID, shared)
				}
			case ZeroIsolated:
				if pipeline.IsTrivialQuery(c.Query) {
					return fmt.Errorf("%s: isolated query is trivial", c.ID)
				}
				if len(sharedTokens(c)) == 0 {
					return fmt.Errorf("%s: isolated query shares no token, so a missing filter would still return nothing", c.ID)
				}
			default:
				return fmt.Errorf("%s: zero reason %q", c.ID, c.ZeroReason)
			}
		} else {
			if c.ZeroReason != "" {
				return fmt.Errorf("%s: zero reason set on a positive case", c.ID)
			}
			if len(c.MustContain) == 0 {
				return fmt.Errorf("%s: needs must_contain", c.ID)
			}
		}
		if c.CustomerScoped && store.CustomerKeyForLabel(c.AskLabel) == "" {
			return fmt.Errorf("%s: customer scope needs a channel label", c.ID)
		}
		blob := strings.ToLower(c.Title + "\n" + c.Query)
		for _, n := range c.Notes {
			blob += "\n" + strings.ToLower(n.Body)
		}
		if emailRE.MatchString(blob) || phoneRE.MatchString(blob) {
			return fmt.Errorf("%s: email or long digit run", c.ID)
		}
		for _, b := range banned {
			if strings.Contains(blob, b) {
				return fmt.Errorf("%s: banned sample text %q", c.ID, b)
			}
		}
	}
	for _, g := range Groups {
		if counts[g.ID] == 0 {
			return fmt.Errorf("group %s is empty", g.ID)
		}
	}
	if neverSaid < 3 {
		return fmt.Errorf("want at least 3 never-said cases, got %d", neverSaid)
	}
	return nil
}

func groupName(id string) string {
	for _, g := range Groups {
		if g.ID == id {
			return g.Name
		}
	}
	return ""
}

// foldVi strips Vietnamese diacritics the way a unicode61 tokenizer does, so a
// never-said case cannot pass validation when SQLite FTS would still match.
func foldVi(s string) string {
	return strings.NewReplacer(
		"à", "a", "á", "a", "ả", "a", "ã", "a", "ạ", "a",
		"ă", "a", "ằ", "a", "ắ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
		"â", "a", "ầ", "a", "ấ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a",
		"è", "e", "é", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e",
		"ê", "e", "ề", "e", "ế", "e", "ể", "e", "ễ", "e", "ệ", "e",
		"ì", "i", "í", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
		"ò", "o", "ó", "o", "ỏ", "o", "õ", "o", "ọ", "o",
		"ô", "o", "ồ", "o", "ố", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
		"ơ", "o", "ờ", "o", "ớ", "o", "ở", "o", "ỡ", "o", "ợ", "o",
		"ù", "u", "ú", "u", "ủ", "u", "ũ", "u", "ụ", "u",
		"ư", "u", "ừ", "u", "ứ", "u", "ử", "u", "ữ", "u", "ự", "u",
		"ỳ", "y", "ý", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y",
		"đ", "d",
	).Replace(strings.ToLower(s))
}

func sharedFoldedTokens(c Case) []string {
	notes := map[string]bool{}
	for _, n := range c.Notes {
		for _, tok := range store.RecallTokens(n.Body) {
			notes[foldVi(tok)] = true
		}
	}
	var hit []string
	seen := map[string]bool{}
	for _, tok := range store.RecallTokens(c.Query) {
		folded := foldVi(tok)
		if notes[folded] && !seen[folded] {
			seen[folded] = true
			hit = append(hit, tok)
		}
	}
	return hit
}

func sharedTokens(c Case) []string {
	var bodies []string
	for _, n := range c.Notes {
		bodies = append(bodies, strings.ToLower(n.Body))
	}
	var hit []string
	for _, tok := range store.RecallTokens(c.Query) {
		for _, body := range bodies {
			if strings.Contains(body, tok) {
				hit = append(hit, tok)
				break
			}
		}
	}
	return hit
}

// agentLabel is a role title, not a person.
func agentLabel(logical string) string {
	switch logical {
	case "legal":
		return "Pháp chế Minh An"
	default:
		return "Cố vấn Minh An"
	}
}

func logicalAgent(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "sales"
	}
	return name
}

func (c Case) tenantOf(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return c.Tenant
}

func (c Case) askTenant() string { return c.tenantOf(c.AskTenant) }

func (c Case) askAgent() string { return logicalAgent(c.AskAgent) }
