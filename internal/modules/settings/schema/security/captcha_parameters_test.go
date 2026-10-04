package settingssecurity

import (
	"github.com/dujiao-next/internal/config"
	"testing"
)

func TestCaptchaPatchPreservesFourAndRejectsInvalidParameters(t *testing.T) {
	current := DefaultCaptchaSetting(config.CaptchaConfig{})
	for _, n := range []int{4, 5, 6, 7, 8} {
		next, err := ApplyCaptchaSettingPatch(current, CaptchaSettingPatch{Image: &CaptchaImagePatch{Length: &n}})
		if err != nil || next.Image.Length != n {
			t.Fatalf("length %d got %d err %v", n, next.Image.Length, err)
		}
	}
	cases := []CaptchaImagePatch{}
	for _, n := range []int{-1, 0, 3, 9} {
		v := n
		cases = append(cases, CaptchaImagePatch{Length: &v})
	}
	for _, n := range []int{99, 601} {
		v := n
		cases = append(cases, CaptchaImagePatch{Width: &v})
	}
	for _, n := range []int{39, 201} {
		v := n
		cases = append(cases, CaptchaImagePatch{Height: &v})
	}
	for _, n := range []int{-1, 21} {
		v := n
		cases = append(cases, CaptchaImagePatch{NoiseCount: &v})
	}
	for _, n := range []int{-1, 1, 3, 15} {
		v := n
		cases = append(cases, CaptchaImagePatch{ShowLine: &v})
	}
	for _, n := range []int{29, 3601} {
		v := n
		cases = append(cases, CaptchaImagePatch{ExpireSeconds: &v})
	}
	for _, n := range []int{99, 100001} {
		v := n
		cases = append(cases, CaptchaImagePatch{MaxStore: &v})
	}
	unknown := "unknown"
	cases = append(cases, CaptchaImagePatch{CharacterType: &unknown})
	for _, p := range cases {
		if _, err := ApplyCaptchaSettingPatch(current, CaptchaSettingPatch{Image: &p}); err == nil {
			t.Fatalf("invalid patch accepted: %+v", p)
		}
	}
	for _, mask := range []int{0, 2, 4, 6, 8, 10, 12, 14} {
		if _, err := ApplyCaptchaSettingPatch(current, CaptchaSettingPatch{Image: &CaptchaImagePatch{ShowLine: &mask}}); err != nil {
			t.Fatal(err)
		}
	}
}
