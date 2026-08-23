package membership_test

import (
	"errors"
	"testing"

	"github.com/smetroid/d3d-api/app/auth/membership"
	"github.com/smetroid/d3d-api/app/models"
)

// fakeDB implements membership.DB with configurable return values.
type fakeDB struct {
	companyIds []string
	groupIds   []string
	err        error
}

func (f *fakeDB) GetUserCompanyIds(_ string) ([]string, error) { return f.companyIds, f.err }
func (f *fakeDB) GetUserGroupIds(_ string) ([]string, error)   { return f.groupIds, f.err }

func TestUserInAudience_Public(t *testing.T) {
	ok, err := membership.UserInAudience(nil, "alice", models.AudienceSpec{Kind: "public"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("public audience should always be true")
	}
}

func TestUserInAudience_User(t *testing.T) {
	audience := models.AudienceSpec{Kind: "user", Ids: []string{"alice", "bob"}}

	ok, err := membership.UserInAudience(nil, "alice", audience)
	if err != nil || !ok {
		t.Errorf("alice should be in user audience: ok=%v err=%v", ok, err)
	}

	ok, err = membership.UserInAudience(nil, "carol", audience)
	if err != nil || ok {
		t.Errorf("carol should not be in user audience: ok=%v err=%v", ok, err)
	}
}

func TestUserInAudience_Company_Match(t *testing.T) {
	db := &fakeDB{companyIds: []string{"co1", "co2"}}
	audience := models.AudienceSpec{Kind: "company", Ids: []string{"co2"}}

	ok, err := membership.UserInAudience(db, "alice", audience)
	if err != nil || !ok {
		t.Errorf("alice is in co2, should match: ok=%v err=%v", ok, err)
	}
}

func TestUserInAudience_Company_NoMatch(t *testing.T) {
	db := &fakeDB{companyIds: []string{"co1"}}
	audience := models.AudienceSpec{Kind: "company", Ids: []string{"co2"}}

	ok, err := membership.UserInAudience(db, "alice", audience)
	if err != nil || ok {
		t.Errorf("alice is not in co2, should not match: ok=%v err=%v", ok, err)
	}
}

func TestUserInAudience_Company_DBError(t *testing.T) {
	dbErr := errors.New("db down")
	db := &fakeDB{err: dbErr}
	audience := models.AudienceSpec{Kind: "company", Ids: []string{"co1"}}

	_, err := membership.UserInAudience(db, "alice", audience)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected db error, got %v", err)
	}
}

func TestUserInAudience_Group_Match(t *testing.T) {
	db := &fakeDB{groupIds: []string{"grp1", "grp2"}}
	audience := models.AudienceSpec{Kind: "group", Ids: []string{"grp2"}}

	ok, err := membership.UserInAudience(db, "alice", audience)
	if err != nil || !ok {
		t.Errorf("alice is in grp2, should match: ok=%v err=%v", ok, err)
	}
}

func TestUserInAudience_Group_NoMatch(t *testing.T) {
	db := &fakeDB{groupIds: []string{"grp1"}}
	audience := models.AudienceSpec{Kind: "group", Ids: []string{"grp2"}}

	ok, err := membership.UserInAudience(db, "alice", audience)
	if err != nil || ok {
		t.Errorf("alice is not in grp2, should not match: ok=%v err=%v", ok, err)
	}
}

func TestUserInAudience_Group_DBError(t *testing.T) {
	dbErr := errors.New("db down")
	db := &fakeDB{err: dbErr}
	audience := models.AudienceSpec{Kind: "group", Ids: []string{"grp1"}}

	_, err := membership.UserInAudience(db, "alice", audience)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected db error, got %v", err)
	}
}

func TestUserInAudience_UnknownKind(t *testing.T) {
	ok, err := membership.UserInAudience(nil, "alice", models.AudienceSpec{Kind: "unknown"})
	if err != nil || ok {
		t.Errorf("unknown kind should return false, nil: ok=%v err=%v", ok, err)
	}
}
