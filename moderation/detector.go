package moderation

import (
	"regexp"
	"strings"
)

type ViolationType string

const (
	ViolationPhone   ViolationType = "phone"
	ViolationLink    ViolationType = "link"
	ViolationContact ViolationType = "contact"
)

type Violation struct {
	Type  ViolationType
	Match string
}

var (
	phonePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(\+7|8)[\s\-\(]*\d{3}[\s\-\)]*\d{3}[\s\-]*\d{2}[\s\-]*\d{2}`),
		regexp.MustCompile(`(?i)\b\d{10,11}\b`),
	}

	// Любые ссылки
	urlPattern = regexp.MustCompile(`(?i)https?://[^\s]+`)
	// Домены без http (example.com, mail.ru и т.д.)
	domainPattern = regexp.MustCompile(`(?i)\b[a-z0-9][-a-z0-9]*\.(ru|com|net|org|io|me|cc|ly|gl|su|рф|info|biz|xyz|online|site|shop|store)\b[^\s]*`)
	// t.me ссылки на аккаунты
	tmePattern = regexp.MustCompile(`(?i)t\.me/[a-zA-Z0-9_]+`)
)

// Check проверяет текст на нарушения
func Check(text string, allowedDomains []string) *Violation {
	textLower := strings.ToLower(text)

	// 1. Проверка телефонов
	for _, p := range phonePatterns {
		if match := p.FindString(text); match != "" {
			digits := regexp.MustCompile(`\d`).FindAllString(match, -1)
			if len(digits) >= 10 {
				return &Violation{
					Type:  ViolationPhone,
					Match: match,
				}
			}
		}
	}

	// 2. Проверка ссылок с http/https
	if urls := urlPattern.FindAllString(textLower, -1); len(urls) > 0 {
		for _, url := range urls {
			if !isAllowedURL(url, allowedDomains) {
				return &Violation{
					Type:  ViolationLink,
					Match: url,
				}
			}
		}
	}

	// 3. Проверка доменов без http (mail.ru, vk.com и т.д.)
	if domains := domainPattern.FindAllString(textLower, -1); len(domains) > 0 {
		for _, domain := range domains {
			if !isAllowedURL(domain, allowedDomains) {
				return &Violation{
					Type:  ViolationLink,
					Match: domain,
				}
			}
		}
	}

	// 4. Проверка t.me ссылок (удаляем все, кроме если t.me в белом списке)
	if match := tmePattern.FindString(textLower); match != "" {
		if !isAllowedURL("t.me", allowedDomains) {
			return &Violation{
				Type:  ViolationContact,
				Match: match,
			}
		}
	}

	return nil
}

func isAllowedURL(url string, allowedDomains []string) bool {
	urlLower := strings.ToLower(url)
	for _, domain := range allowedDomains {
		if strings.Contains(urlLower, strings.ToLower(domain)) {
			return true
		}
	}
	return false
}
