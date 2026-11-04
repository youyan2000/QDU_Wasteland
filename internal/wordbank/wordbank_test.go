package wordbank

import "testing"

func TestCountIsNonTrivial(t *testing.T) {
	if Count() < 1000 {
		t.Fatalf("内置词库过小（%d），疑似生成失败", Count())
	}
}

func TestPickReturnsDistinctWordsWithDefinition(t *testing.T) {
	words := Pick(2)
	if len(words) != 2 {
		t.Fatalf("期望 2 个词，实际 %d", len(words))
	}
	if words[0].W == words[1].W {
		t.Fatal("同一次取词不应重复")
	}
	for _, w := range words {
		if w.W == "" || w.EN == "" {
			t.Fatalf("词条缺少单词或英文释义: %+v", w)
		}
	}
}

func TestPickClampsToBankSize(t *testing.T) {
	if got := len(Pick(1)); got != 1 {
		t.Fatalf("Pick(1) 返回 %d 条", got)
	}
	if got := Pick(0); got != nil {
		t.Fatalf("Pick(0) 应返回 nil，实际 %d 条", len(got))
	}
	if got := Pick(-3); got != nil {
		t.Fatalf("负数量应返回 nil，实际 %d 条", len(got))
	}
}
