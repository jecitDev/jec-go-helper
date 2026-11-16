package datachangelog

import "strings"

// findEntityConfig mengembalikan konfigurasi entity pertama yang domain-nya cocok.
func findEntityConfig(cfg *Config, domain string) *EntityConfig {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Entities {
		if cfg.Entities[i].Domain == domain {
			return &cfg.Entities[i]
		}
	}
	return nil
}

// resolveOperation menentukan operasi untuk sebuah method gRPC dan apakah method itu
// boleh dicatat menurut config entity.
//
// Urutan:
//  1. Alias eksplisit di config, format "OPERASI:NamaMethod" (contoh "CREATE:CreatePolyclinic").
//     Method yang namanya cocok persis (tidak peka huruf besar/kecil) memakai operasi dari alias.
//  2. Entri biasa tanpa ":" (contoh "UPDATE") dicocokkan dengan operasi yang ditebak
//     dari nama method lewat inferOperation (perilaku lama, tetap berlaku).
//
// Method yang tidak cocok dengan keduanya dikembalikan allowed=false.
func resolveOperation(entityCfg *EntityConfig, methodName string) (operation string, allowed bool) {
	inferred := inferOperation(methodName)
	if entityCfg == nil {
		return inferred, false
	}

	for _, entry := range entityCfg.Operations {
		op, method, hasAlias := strings.Cut(entry, ":")
		if hasAlias && strings.EqualFold(strings.TrimSpace(method), methodName) {
			return strings.ToUpper(strings.TrimSpace(op)), true
		}
	}

	for _, entry := range entityCfg.Operations {
		if !strings.Contains(entry, ":") && strings.EqualFold(strings.TrimSpace(entry), inferred) {
			return inferred, true
		}
	}

	return inferred, false
}
