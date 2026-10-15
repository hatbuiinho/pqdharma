package phonetics

import "testing"

func TestOxfordStressNotation(t *testing.T) {
	cases := []struct{ word, raw, want string }{
		{"right", "rˈaɪt", "raɪt"},
		{"I", "ˈaɪ", "aɪ"},
		{"the", "ðˈə", "ðə"},
		{"thus", "ðˈʌs", "ðʌs"},
		{"deer", "dˈiə", "diə"},
		{"near", "nˈɪə", "nɪə"},
		{"poor", "pˈʊə", "pʊə"},
		{"about", "əbˈaʊt", "əˈbaʊt"},
		{"ability", "əbˈɪləti", "əˈbɪləti"},
		{"important", "ɪmpˈɔːtənt", "ɪmˈpɔːtənt"},
		{"endeavour", "endˈevə", "enˈdevə"},
		{"afraid", "əfrˈeɪd", "əˈfreɪd"},
		{"agree", "əgrˈiː", "əˈgriː"},
		{"extraordinary", "ɪkstrˈɔːdənəri", "ɪkˈstrɔːdənəri"},
		{"noble", "nˈəʊbəl", "ˈnəʊbəl"},
		{"venerable", "vˈenərəbəl", "ˈvenərəbəl"},
		{"meditative", "mˈedɪtˌeɪtɪv", "ˈmedɪteɪtɪv"},
		{"disagree", "dˌɪsəgrˈiː", "ˌdɪsəˈgriː"},
		{"understand", "ˌʌndərstˈænd", "ˌʌndərˈstænd"},
		{"abbreviation", "əbrˌiːviˈeɪʃn", "əˌbriːviˈeɪʃn"},
		{"agricultural", "ˌægrɪkˈʌltʃərəl", "ˌægrɪˈkʌltʃərəl"},
		{"glorious", "glˈɔːriəs", "ˈglɔːriəs"},
		{"button", "bˈʌtn", "ˈbʌtn"},
		{"rhythm", "rˈɪðm", "ˈrɪðm"},
		{"right about", "rˈaɪt əbˈaʊt", "raɪt əˈbaʊt"},
		{"about", "əˈbaʊt", "əˈbaʊt"}, // Already syllable-initial.
		{"ability", "əˈbɪləti", "əˈbɪləti"},
		{"agricultural", "ˌægrɪˈkʌltʃərəl", "ˌægrɪˈkʌltʃərəl"},
		{"plain", "plain", "plain"},
	}
	for _, c := range cases {
		t.Run(c.word+"/"+c.raw, func(t *testing.T) {
			got := normalizeStress(c.word, c.raw)
			if got != c.want {
				t.Fatalf("%s: %q => %q, want %q", c.word, c.raw, got, c.want)
			}
			if twice := normalizeStress(c.word, got); twice != got {
				t.Fatalf("not idempotent: %q => %q", got, twice)
			}
		})
	}
}
