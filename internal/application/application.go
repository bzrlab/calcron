// Package application owns application identity and token lifecycle.
package application

import (
	"context"

	"github.com/calcron/calcron/internal/protocol"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Module struct {
	db    *pgxpool.Pool
	newID func() string
}

func New(db *pgxpool.Pool, newID func() string) *Module { return &Module{db: db, newID: newID} }

func (m *Module) Create(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.Name == "" || f.Namespace == "" {
		return fail("name and namespace required")
	}
	id, tokenID, secret := m.newID(), m.newID(), m.newID()+m.newID()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return fail(err.Error())
	}
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return fail(err.Error())
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `insert into applications(id,namespace) values($1,$2)`, id, f.Namespace); err == nil {
		_, err = tx.Exec(ctx, `insert into application_tokens(id,application_id,secret_hash) values($1,$2,$3)`, tokenID, id, string(hash))
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		return fail("application exists")
	}
	return ok(map[string]string{"applicationId": id, "token": "cc_" + tokenID + "_" + secret})
}

func (m *Module) RotateToken(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.ApplicationID == "" {
		return fail("applicationId required")
	}
	tokenID, secret := m.newID(), m.newID()+m.newID()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return fail(err.Error())
	}
	tag, err := m.db.Exec(ctx, `insert into application_tokens(id,application_id,secret_hash) select $1,id,$2 from applications where id=$3`, tokenID, string(hash), f.ApplicationID)
	if err != nil {
		return fail(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return fail("application not found")
	}
	return ok(map[string]string{"tokenId": tokenID, "token": "cc_" + tokenID + "_" + secret})
}

func (m *Module) RevokeToken(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.TokenID == "" {
		return fail("tokenId required")
	}
	tag, err := m.db.Exec(ctx, `update application_tokens set revoked_at=now() where id=$1 and revoked_at is null`, f.TokenID)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]bool{"revoked": tag.RowsAffected() == 1})
}

func fail(err string) protocol.Reply { return protocol.Reply{Error: err} }
func ok(data any) protocol.Reply     { return protocol.Reply{OK: true, Data: data} }
