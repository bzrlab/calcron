package server

import (
	"context"

	"golang.org/x/crypto/bcrypt"
)

func (s *Server) createApp(ctx context.Context, f frame) reply {
	if f.Name == "" || f.Namespace == "" {
		return fail("name and namespace required")
	}
	id, tokenID, secret := random(), random(), random()+random()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return fail(err.Error())
	}
	tx, err := s.db.Begin(ctx)
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

func (s *Server) rotateToken(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" {
		return fail("applicationId required")
	}
	tokenID, secret := random(), random()+random()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return fail(err.Error())
	}
	tag, err := s.db.Exec(ctx, `insert into application_tokens(id,application_id,secret_hash) select $1,id,$2 from applications where id=$3`, tokenID, string(hash), f.ApplicationID)
	if err != nil {
		return fail(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return fail("application not found")
	}
	return ok(map[string]string{"tokenId": tokenID, "token": "cc_" + tokenID + "_" + secret})
}

func (s *Server) revokeToken(ctx context.Context, f frame) reply {
	if f.TokenID == "" {
		return fail("tokenId required")
	}
	tag, err := s.db.Exec(ctx, `update application_tokens set revoked_at=now() where id=$1 and revoked_at is null`, f.TokenID)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]bool{"revoked": tag.RowsAffected() == 1})
}
