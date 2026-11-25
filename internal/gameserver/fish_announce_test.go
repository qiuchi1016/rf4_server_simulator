package gameserver

import (
	"bytes"
	"testing"
	"time"

	"rf4server/internal/proto"
	"rf4server/internal/tunnel"
)

// 颜色分档：25=蓝冠及以上、26=奖杯、3=达标；未达标不发模板。
func TestFishAnnouncementKind(t *testing.T) {
	cases := []struct {
		name  string
		grade int64
		want  int
	}{
		{"蓝冠", 4, 25},
		{"奖杯", 3, 26},
		{"达标", 2, 3},
		{"未达标", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fishAnnouncementKind(tc.grade); got != tc.want {
				t.Fatalf("grade=%d: got %d, want %d", tc.grade, got, tc.want)
			}
		})
	}
}

// 三档播报走各自 kind：25=蓝冠（深橘红）、26=奖杯（橘黄）、3=达标（蓝色，无前缀）。
func TestFishLandedUsesGradeKind(t *testing.T) {
	srv, sess, gc := newEscapeProbeServer(t)
	srv.sessions[sess.ID] = sess
	for _, tc := range []struct {
		grade int64
		kind  uint64
	}{
		{4, 25},
		{3, 26},
		{2, 3},
	} {
		srv.notifyFishLanded(sess, &RolledFish{Species: "g_tench", Grade: tc.grade, WeightG: 4000}, false)
		env := awaitPushEnvelope(t, gc)
		if env.Main != 24 || env.Sub != 27 {
			t.Fatalf("grade=%d 应走 24/27，实得 %d/%d", tc.grade, env.Main, env.Sub)
		}
		dec, _, err := proto.DecodeValue(env.Body, 0, srv.Table())
		if err != nil {
			t.Fatalf("grade=%d 推送不可解码: %v", tc.grade, err)
		}
		if f0 := dec.Items[0].Field(0).Num; f0 == nil || *f0 != tc.kind {
			t.Fatalf("grade=%d 应用 kind%d，实得 %v", tc.grade, tc.kind, f0)
		}
	}
}

func TestFishLandedPublicBroadcast(t *testing.T) {
	for _, tc := range []struct {
		name string
		kv   float64
		kind uint64
	}{
		{"模板", 1, 26},
		{"纯文本", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, host, hostConn := newEscapeProbeServer(t)
			friend, friendConn := newTunnelProbe(t, srv)
			friend.ID, friend.Userid = host.ID+1, host.Userid+1
			srv.sessions[host.ID], srv.sessions[friend.ID] = host, friend
			srv.tunCache, srv.tunAt = map[string]float64{"fish_tv_kv": tc.kv}, time.Now()

			for _, angler := range []*Session{host, friend} {
				srv.notifyFishLanded(angler, &RolledFish{Species: "g_tench", Grade: 3, WeightG: 4000}, false)
				for _, conn := range []*tunnel.Conn{hostConn, friendConn} {
					env := awaitPushEnvelope(t, conn)
					if env.Main != 24 || env.Sub != 27 {
						t.Fatalf("公聊收到 %d/%d，期望 24/27", env.Main, env.Sub)
					}
					body, _, err := proto.DecodeValue(env.Body, 0, srv.Table())
					if err != nil {
						t.Fatal(err)
					}
					if len(body.Items) != 3 {
						t.Fatal("渔获公聊载荷必须包含消息、频道和人数")
					}
					if body.Items[1].StrOr() != "" || body.Items[2].U32Val() != 2 {
						t.Fatal("两个玩家的公聊应使用通用频道并显示两人在线")
					}
					msg := body.Items[0]
					if uid := msg.Field(2).Num; uid == nil || *uid != uint64(angler.Userid) {
						t.Fatalf("钓手应为 %d，实得 %v", angler.Userid, uid)
					}
					if kind := msg.Field(0).Num; kind == nil || *kind != tc.kind || !bytes.Contains(env.Body, []byte("g_tench")) {
						t.Fatalf("渔获播报内容有误: %x", env.Body)
					}
				}
			}
			srv.notifyFishLanded(host, &RolledFish{Species: "g_tench", Grade: 1, WeightG: 100}, true)
			assertNoPush(t, hostConn, 100*time.Millisecond, "自己收到重复播报或未达标鱼播报")
			assertNoPush(t, friendConn, 100*time.Millisecond, "朋友收到重复播报或未达标鱼播报")
		})
	}
}

// 入护播报已永久停用：未达标与达标鱼都不再补白色“入护：…”行。
func TestFishKeptDoesNotBroadcast(t *testing.T) {
	srv, sess, gc := newEscapeProbeServer(t)
	srv.notifyFishKept(sess, &RolledFish{FishGUID: "undersized", Species: "g_tench", WeightG: 400, Grade: 1})
	srv.notifyFishKept(sess, &RolledFish{FishGUID: "kept", Species: "g_tench", WeightG: 900, Grade: 2})

	done := make(chan string, 1)
	go func() {
		_, _, payload, err := gc.ReadRecord()
		if err != nil {
			done <- ""
			return
		}
		done <- string(payload)
	}()
	select {
	case payload := <-done:
		if payload != "" {
			t.Fatalf("入护播报应永久停用，仍收到推送: %q", payload)
		}
	case <-time.After(300 * time.Millisecond):
		// 没有推送 = 期望结果
	}
}
