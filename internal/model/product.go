package model

import "time"

type Product struct {
	ID          int       `db:"id"`
	Slug        string    `db:"slug"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	Price       int       `db:"price"` // тиын (тенге × 100)
	Currency    string    `db:"currency"`
	IsActive    bool      `db:"is_active"`
	CreatedAt   time.Time `db:"created_at"`

	Images   []ProductImage
	Variants []ProductVariant
}

func (p *Product) DisplayTitle() string {
	if p.Slug == "besh" {
		return "BESH"
	}
	return "ШАЙ"
}

func (p *Product) DisplayLabel() string {
	if p.Slug == "besh" {
		return "BESH SAVED MY LIFE"
	}
	return "КЕЛ, ШАЙ IШЕМIЗ"
}

func (p *Product) FormattedPrice() string {
	tenge := p.Price / 100
	return formatInt(tenge) + " ₸"
}

func formatInt(n int) string {
	s := ""
	for i, r := range reverse(itoa(n)) {
		if i > 0 && i%3 == 0 {
			s = " " + s
		}
		s = string(r) + s
	}
	return s
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s += string(rune('0' + n%10))
		n /= 10
	}
	return s
}

type ProductImage struct {
	ID        int    `db:"id"`
	ProductID int    `db:"product_id"`
	Side      string `db:"side"`
	Filename  string `db:"filename"`
	SortOrder int    `db:"sort_order"`
}

type ProductVariant struct {
	ID        int    `db:"id"`
	ProductID int    `db:"product_id"`
	Size      string `db:"size"`
	Stock     int    `db:"stock"`
}
