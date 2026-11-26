package gameserver

import (
	"context"
	"testing"

	"rf4server/internal/logging"
	"rf4server/internal/proto"
)

func TestDepthSettingTargetsNamedRodBeforeFirstCast(t *testing.T) {
	s, db := newRigTestServer(t)
	sess := newProbeSess(s)
	ctx := context.Background()
	first, err := s.CreateItem(ctx, sess.Userid, "tele_10012_tl16", "backpack", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateItem(ctx, sess.Userid, "tele_10012_tl16", "backpack", 1)
	if err != nil {
		t.Fatal(err)
	}
	held := s.selectFishingRod(sess.Userid, first.GUID)
	set := func(guid string, depth uint32) {
		t.Helper()
		if _, ok := hRodSetDepth(&Ctx{Sess: sess, Main: 17, Sub: 12, Tree: rigRequestTree(t, proto.Guid(guid), proto.U32(depth)), DecodeOK: true, Log: logging.Discard()}); !ok {
			t.Fatal("17/12 was not handled")
		}
	}
	set(first.GUID, 180)
	set(second.GUID, 18)
	if s.stateFor(sess.Userid) != held {
		t.Fatal("setting the other rod changed the held rod")
	}
	if held.DepthCM != 180 {
		t.Errorf("held rod depth = %d, want 180", held.DepthCM)
	}
	if got := s.fishingStateForRod(sess.Userid, second.GUID).DepthCM; got != 18 {
		t.Errorf("second rod depth = %d, want 18", got)
	}
	if got := storedRigField(t, db, second.GUID, 4); got != 18 {
		t.Errorf("stored depth = %d, want 18", got)
	}
}

func TestCastRestoresDepthFromFloatRod(t *testing.T) {
	for _, step := range []struct {
		name      string
		main, sub int
		handler   func(*Ctx) ([]byte, bool)
	}{
		{"take", 17, 7, hRodTake},
		{"prepare", 14, 1, hCastPrepare},
		{"cast", 14, 2, hCast},
	} {
		t.Run(step.name, func(t *testing.T) {
			s, _ := newRigTestServer(t)
			sess := newProbeSess(s)
			rod, err := s.CreateItem(context.Background(), sess.Userid, "tele_10012_tl16", "backpack", 1)
			if err != nil {
				t.Fatal(err)
			}
			st := s.selectFishingRod(sess.Userid, rod.GUID)
			_, _ = hRodSetDepth(&Ctx{Sess: sess, Main: 17, Sub: 12, Tree: rigRequestTree(t, proto.Guid(rod.GUID), proto.U32(18)), DecodeOK: true, Log: logging.Discard()})
			// Simulate an older fishing_state snapshot that disagrees with the rod's f5.
			st.mu.Lock()
			st.DepthCM = 300
			st.mu.Unlock()
			body, handled := step.handler(&Ctx{Sess: sess, Main: step.main, Sub: step.sub, Tree: rigRequestTree(t, proto.Guid(rod.GUID)), DecodeOK: true, ExpectResponse: true, Log: logging.Discard()})
			if !handled || len(body) == 0 {
				t.Fatal("missing cast preparation response")
			}
			if got := st.DepthCM; got != 18 {
				t.Fatalf("cast used stale depth %d, want 18", got)
			}
			if step.sub != 2 {
				tree, _, err := proto.DecodeValue(body, 0, s.Table())
				if err != nil {
					t.Fatal(err)
				}
				if got := tree.Fields[2].U32(); got != proto.F32Bits(0.18) {
					t.Fatalf("depth sent in 158.f2 = %#x, want 0.18 m", got)
				}
			}
		})
	}
}

func TestDepthWriteFailurePreservesRodAndSnapshot(t *testing.T) {
	s, db := newRigTestServer(t)
	sess := newProbeSess(s)
	ctx := context.Background()
	rod, err := s.CreateItem(ctx, sess.Userid, "tele_10012_tl16", "backpack", 1)
	if err != nil {
		t.Fatal(err)
	}
	st := s.selectFishingRod(sess.Userid, rod.GUID)
	set := func(depth uint32) {
		_, _ = hRodSetDepth(&Ctx{Sess: sess, Main: 17, Sub: 12,
			Tree: rigRequestTree(t, proto.Guid(rod.GUID), proto.U32(depth)), Log: logging.Discard()})
	}
	set(180)
	if _, err := db.SQL.ExecContext(ctx, "CREATE TRIGGER reject_depth BEFORE UPDATE OF fields_json ON item BEGIN SELECT RAISE(ABORT, 'simulated write failure'); END"); err != nil {
		t.Fatal(err)
	}
	set(18)
	if got := storedRigField(t, db, rod.GUID, 4); got != 180 {
		t.Errorf("rod f5 depth = %d, want 180", got)
	}
	if got := st.DepthCM; got != 180 {
		t.Errorf("runtime depth = %d despite failed f5 write, want 180", got)
	}
}

func TestCastPrepareRecoversLegacyFloatDepthWithoutRigObject(t *testing.T) {
	s, db := newRigTestServer(t)
	sess := newProbeSess(s)
	ctx := context.Background()
	rod, err := s.CreateItem(ctx, sess.Userid, "tele_10012_tl16", "backpack", 1)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := FieldsFromJSON(rod.FieldsJSON)
	if err != nil {
		t.Fatal(err)
	}
	fields[fieldIdxByCS(rod.WireClass, csRigObject)].Val = proto.Null()
	if err := s.saveItemFields(ctx, rod, fields); err != nil {
		t.Fatal(err)
	}
	st := s.selectFishingRod(sess.Userid, rod.GUID)
	st.mu.Lock()
	st.DepthCM = 18
	st.mu.Unlock()
	_, ok := hCastPrepare(&Ctx{Sess: sess, Main: 14, Sub: 1, Tree: rigRequestTree(t, proto.Guid(rod.GUID)), DecodeOK: true, Log: logging.Discard()})
	if !ok || st.DepthCM != 18 {
		t.Fatalf("legacy snapshot depth = %d, want 18", st.DepthCM)
	}
	if got := storedRigField(t, db, rod.GUID, 4); got != 18 {
		t.Errorf("recovered f5 depth = %d, want 18", got)
	}
}

func TestFirstCastUsesCorrectedFloatRig(t *testing.T) {
	for _, step := range []struct {
		name      string
		main, sub int
		handler   func(*Ctx) ([]byte, bool)
	}{{"take", 17, 7, hRodTake}, {"prepare", 14, 1, hCastPrepare}, {"cast", 14, 2, hCast}} {
		t.Run(step.name, func(t *testing.T) {
			s, db := newRigTestServer(t)
			sess := newProbeSess(s)
			ctx := context.Background()
			rod, err := s.CreateItem(ctx, sess.Userid, "tele_10012_tl16", "backpack", 1)
			if err != nil {
				t.Fatal(err)
			}
			fields, err := FieldsFromJSON(rod.FieldsJSON)
			if err != nil {
				t.Fatal(err)
			}
			idx := fieldIdxByCS(rod.WireClass, csRigObject)
			fields[idx].Val = buildRig153("rig_BottomSimple", 12, nil, 18)
			if err := s.saveItemFields(ctx, rod, fields); err != nil {
				t.Fatal(err)
			}
			st := s.selectFishingRod(sess.Userid, rod.GUID)
			_, ok := step.handler(&Ctx{Sess: sess, Main: step.main, Sub: step.sub, Tree: rigRequestTree(t, proto.Guid(rod.GUID)), DecodeOK: true, Log: logging.Discard()})
			if !ok {
				t.Fatal("unhandled request")
			}
			if got := storedRigField(t, db, rod.GUID, 1); got != 1 {
				t.Fatalf("stored corrected type = %d, want 1", got)
			}
			if st.RigType != 1 || st.DepthCM != 18 {
				t.Errorf("first cast uses rig %d at %d cm, want rig 1 at 18 cm", st.RigType, st.DepthCM)
			}
		})
	}
}
