package textutil

import "testing"

func TestParseSalaryRange(t *testing.T) {
	min, max, cur, ok := ParseSalary("Compensation: $120,000 - $150,000 per year")
	if !ok || min != 120000 || max != 150000 || cur != "USD" {
		t.Fatalf("got %d %d %s ok=%v", min, max, cur, ok)
	}
	min, max, cur, ok = ParseSalary("Pay is $120k-$150k")
	if !ok || min != 120000 || max != 150000 || cur != "USD" {
		t.Fatalf("k range got %d %d %s ok=%v", min, max, cur, ok)
	}
	min, max, cur, ok = ParseSalary("30-36 USD hourly")
	if !ok || min != 30 || max != 36 || cur != "USD" {
		t.Fatalf("hourly got %d %d %s ok=%v", min, max, cur, ok)
	}
	min, max, cur, ok = ParseSalary("The salary is £50,000.")
	if !ok || min != 50000 || max != 50000 || cur != "GBP" {
		t.Fatalf("single got %d %d %s ok=%v", min, max, cur, ok)
	}
}

func TestParseSalaryIgnoresYears(t *testing.T) {
	if _, _, _, ok := ParseSalary("Worked from 2020-2024 on backend systems."); ok {
		t.Fatal("years should not be pay")
	}
	if _, _, _, ok := ParseSalary("No compensation listed"); ok {
		t.Fatal("expected no match")
	}
	if _, _, _, ok := ParseSalary("We match up to $2000 for donations."); ok {
		t.Fatal("donation match is not pay")
	}
}

func TestParseSalaryEuropeanThousands(t *testing.T) {
	min, max, cur, ok := ParseSalary("Hiring Ranges: Remote Germany €68.000 — €91.000 EUR")
	if !ok || min != 68000 || max != 91000 || cur != "EUR" {
		t.Fatalf("got %d %d %s ok=%v", min, max, cur, ok)
	}
	min, max, cur, ok = ParseSalary("US$1,000 per task, and you can do as many as you want.")
	if !ok || min != 1000 || max != 1000 || cur != "USD" {
		t.Fatalf("task pay got %d %d %s ok=%v", min, max, cur, ok)
	}
}
