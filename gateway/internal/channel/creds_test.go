// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"testing"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestChannelCred_EnvWinsBox(t *testing.T) {
	st := store.New()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	boxName := SecretName("zalo-personal", kindSession)
	if err := secrets.Put(st, boxName, []byte("from-box")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	v, fromEnv, set := Credential(st, "zalo-personal", kindSession, []string{"GOSO_ZALO_PERSONAL_TOKEN"})
	if !set || fromEnv || v != "from-box" {
		t.Fatalf("box %+v %v %v", v, fromEnv, set)
	}
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "from-env")
	v, fromEnv, set = Credential(st, "zalo-personal", kindSession, []string{"GOSO_ZALO_PERSONAL_TOKEN"})
	if !set || !fromEnv || v != "from-env" {
		t.Fatalf("env wins %+v %v %v", v, fromEnv, set)
	}
	if err := secrets.Delete(st, boxName); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	if SecretSet(st, "zalo-personal", kindSession, []string{"GOSO_ZALO_PERSONAL_TOKEN"}) {
		t.Fatal("cleared")
	}
}

func TestSecretName(t *testing.T) {
	if SecretName("telegram", "bot") != "channel:telegram:bot" {
		t.Fatal(SecretName("telegram", "bot"))
	}
}

func TestWritableFieldsAndFieldKind(t *testing.T) {
	if got := WritableFields("telegram"); len(got) != 1 || got[0] != "bot_token" {
		t.Fatalf("telegram %v", got)
	}
	if got := WritableFields("zalo-oa"); len(got) != 2 {
		t.Fatalf("oa %v", got)
	}
	if WritableFields("zalo-personal") != nil || WritableFields("discord") != nil {
		t.Fatal("personal/phase2 must have no token form")
	}
	k, ok := FieldKind("telegram", "bot_token")
	if !ok || k != KindBot {
		t.Fatal("bot kind")
	}
	if _, ok := FieldKind("telegram", "access_token"); ok {
		t.Fatal("telegram access")
	}
}
