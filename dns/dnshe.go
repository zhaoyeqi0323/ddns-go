package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jeessy2/ddns-go/v6/config"
	"github.com/jeessy2/ddns-go/v6/util"
)

// DNSHE Free Domain API V2.0
// 文档: https://my.dnshe.com/knowledgebase/13/DNSHE-Free-Domain-API-User-Guide-V2.0.html
// 认证方式: 在 HTTP Header 中传递 X-API-Key / X-API-Secret
const (
	dnsheBaseURL          = "https://api005.dnshe.com/index.php?m=domain_hub"
	dnsheListSubdomainAPI = dnsheBaseURL + "&endpoint=subdomains&action=list"
	dnsheListRecordAPI    = dnsheBaseURL + "&endpoint=dns_records&action=list"
	dnsheCreateRecordAPI  = dnsheBaseURL + "&endpoint=dns_records&action=create"
	dnsheUpdateRecordAPI  = dnsheBaseURL + "&endpoint=dns_records&action=update"
)

// DNSHE 实现 ddns-go 的 DNS 接口
type DNSHE struct {
	DNS              config.DNS
	Domains          config.Domains
	TTL              int
	httpClient       *http.Client
	subdomains       []dnsheSubdomain
	subdomainsLoaded bool
}

type dnsheSubdomain struct {
	ID         int    `json:"id"`
	Subdomain  string `json:"subdomain"`
	RootDomain string `json:"rootdomain"`
	FullDomain string `json:"full_domain"`
	Status     string `json:"status"`
}

type dnsheRecord struct {
	ID       int    `json:"id"`
	RecordID string `json:"record_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Status   string `json:"status"`
}

type dnsheSubdomainsResp struct {
	Success    bool             `json:"success"`
	Message    string           `json:"message"`
	Count      int              `json:"count"`
	Subdomains []dnsheSubdomain `json:"subdomains"`
}

type dnsheRecordsResp struct {
	Success bool          `json:"success"`
	Message string        `json:"message"`
	Count   int           `json:"count"`
	Records []dnsheRecord `json:"records"`
}

type dnsheActionResp struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	ID      int    `json:"id"`
}

type dnsheRecordRequest struct {
	SubdomainID int    `json:"subdomain_id,omitempty"`
	ID          int    `json:"id,omitempty"`
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Content     string `json:"content"`
	TTL         int    `json:"ttl,omitempty"`
}

// Init 初始化
func (dnshe *DNSHE) Init(dnsConf *config.DnsConfig, ipv4cache *util.IpCache, ipv6cache *util.IpCache) {
	dnshe.Domains.Ipv4Cache = ipv4cache
	dnshe.Domains.Ipv6Cache = ipv6cache
	dnshe.DNS = dnsConf.DNS
	dnshe.Domains.GetNewIp(dnsConf)
	if dnsConf.TTL == "" {
		// 默认 300s
		dnshe.TTL = 300
	} else {
		ttl, err := strconv.Atoi(dnsConf.TTL)
		if err != nil {
			dnshe.TTL = 300
		} else {
			dnshe.TTL = ttl
		}
	}
	dnshe.httpClient = dnsConf.GetHTTPClient()
}

// AddUpdateDomainRecords 添加或更新 IPv4/IPv6 记录
func (dnshe *DNSHE) AddUpdateDomainRecords() config.Domains {
	dnshe.addUpdateDomainRecords("A")
	dnshe.addUpdateDomainRecords("AAAA")
	return dnshe.Domains
}

func (dnshe *DNSHE) addUpdateDomainRecords(recordType string) {
	ipAddr, domains := dnshe.Domains.GetNewIpResult(recordType)
	if ipAddr == "" {
		return
	}

	for _, domain := range domains {
		subID, subFullDomain, err := dnshe.getSubdomainID(domain)
		if err != nil {
			util.Log("DNSHE 获取子域名信息失败 %s! %s", domain, err)
			domain.UpdateStatus = config.UpdatedFailed
			continue
		}

		var listResp dnsheRecordsResp
		err = dnshe.request("GET", dnsheListRecordAPI+"&subdomain_id="+strconv.Itoa(subID), nil, &listResp)
		if err != nil {
			util.Log("DNSHE 查询域名解析失败 %s! %s", domain, err)
			domain.UpdateStatus = config.UpdatedFailed
			continue
		}
		if !listResp.Success {
			util.Log("DNSHE 查询域名解析失败 %s! %s", domain, listResp.Message)
			domain.UpdateStatus = config.UpdatedFailed
			continue
		}

		find := false
		for _, record := range listResp.Records {
			if record.Type == recordType && recordNameMatches(record.Name, domain) {
				// 已存在则更新
				dnshe.modify(record, domain, ipAddr)
				find = true
				break
			}
		}
		if !find {
			// 不存在则创建
			dnshe.create(subID, subFullDomain, domain, recordType, ipAddr)
		}
	}
}

// getSubdomainID 根据 ddns-go 域名找到 DNSHE 子域名的内部 ID
// 通过最长后缀匹配, 兼容 apex (home.cc.cd) 与子标签 (www.home.cc.cd) 两种写法
func (dnshe *DNSHE) getSubdomainID(domain *config.Domain) (int, string, error) {
	if !dnshe.subdomainsLoaded {
		var resp dnsheSubdomainsResp
		err := dnshe.request("GET", dnsheListSubdomainAPI, nil, &resp)
		if err != nil {
			return 0, "", err
		}
		if !resp.Success {
			return 0, "", fmt.Errorf("%s", resp.Message)
		}
		dnshe.subdomains = resp.Subdomains
		dnshe.subdomainsLoaded = true
	}

	full := domain.String()
	bestID := 0
	bestFull := ""
	bestLen := -1
	for _, sub := range dnshe.subdomains {
		if sub.FullDomain == "" || sub.Status != "active" {
			continue
		}
		if full == sub.FullDomain || strings.HasSuffix(full, "."+sub.FullDomain) {
			if len(sub.FullDomain) > bestLen {
				bestLen = len(sub.FullDomain)
				bestID = sub.ID
				bestFull = sub.FullDomain
			}
		}
	}
	if bestID == 0 {
		return 0, "", fmt.Errorf("未找到匹配的 DNSHE 子域名 (full_domain=%s)", full)
	}
	return bestID, bestFull, nil
}

// create 创建新的解析记录
func (dnshe *DNSHE) create(subdomainID int, subFullDomain string, domain *config.Domain, recordType string, ipAddr string) {
	body := dnsheRecordRequest{
		SubdomainID: subdomainID,
		Type:        recordType,
		Name:        relativeName(domain.String(), subFullDomain),
		Content:     ipAddr,
		TTL:         dnshe.TTL,
	}
	var resp dnsheActionResp
	err := dnshe.request("POST", dnsheCreateRecordAPI, body, &resp)
	if err != nil {
		util.Log("DNSHE 新增域名解析 %s 失败! 异常信息: %s", domain, err)
		domain.UpdateStatus = config.UpdatedFailed
		return
	}
	if !resp.Success {
		util.Log("DNSHE 新增域名解析 %s 失败! %s", domain, resp.Message)
		domain.UpdateStatus = config.UpdatedFailed
		return
	}
	util.Log("DNSHE 新增域名解析 %s 成功! IP: %s", domain, ipAddr)
	domain.UpdateStatus = config.UpdatedSuccess
}

// modify 更新已有解析记录
func (dnshe *DNSHE) modify(record dnsheRecord, domain *config.Domain, ipAddr string) {
	// 没有变化直接跳过
	if record.Content == ipAddr {
		util.Log("你的IP %s 没有变化, 域名 %s", ipAddr, domain)
		return
	}
	body := dnsheRecordRequest{
		ID:      record.ID,
		Type:    record.Type,
		Content: ipAddr,
		TTL:     dnshe.TTL,
	}
	var resp dnsheActionResp
	err := dnshe.request("POST", dnsheUpdateRecordAPI, body, &resp)
	if err != nil {
		util.Log("DNSHE 更新域名解析 %s 失败! 异常信息: %s", domain, err)
		domain.UpdateStatus = config.UpdatedFailed
		return
	}
	if !resp.Success {
		util.Log("DNSHE 更新域名解析 %s 失败! %s", domain, resp.Message)
		domain.UpdateStatus = config.UpdatedFailed
		return
	}
	util.Log("DNSHE 更新域名解析 %s 成功! IP: %s", domain, ipAddr)
	domain.UpdateStatus = config.UpdatedSuccess
}

// request 统一请求接口 (Header 注入 API Key / Secret)
func (dnshe *DNSHE) request(method string, url string, data interface{}, result interface{}) (err error) {
	jsonStr := make([]byte, 0)
	if data != nil {
		jsonStr, err = json.Marshal(data)
		if err != nil {
			return
		}
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return
	}
	req.Header.Set("X-API-Key", dnshe.DNS.ID)
	req.Header.Set("X-API-Secret", dnshe.DNS.Secret)
	req.Header.Set("Content-Type", "application/json")

	client := dnshe.httpClient
	resp, err := client.Do(req)
	err = util.GetHTTPResponse(resp, err, result)
	return
}

// recordNameMatches 判断 DNSHE 记录名是否匹配 ddns-go 域名
// DNSHE 列表返回的 name 可能是完整域名, 也可能是相对名 (@ 或子标签)
func recordNameMatches(recordName string, domain *config.Domain) bool {
	if recordName == domain.String() {
		return true
	}
	if recordName == "@" || recordName == "" {
		return domain.GetSubDomain() == "@"
	}
	if domain.GetSubDomain() != "@" && recordName == domain.GetSubDomain() {
		return true
	}
	return false
}

// relativeName 计算相对 DNSHE 子域名的记录名
// 例如 full=www.home.cc.cd, subFullDomain=home.cc.cd -> "www"; apex -> "@"
func relativeName(full string, subFullDomain string) string {
	if full == subFullDomain {
		return "@"
	}
	prefix := strings.TrimSuffix(full, "."+subFullDomain)
	if prefix == "" || prefix == full {
		return "@"
	}
	return prefix
}
