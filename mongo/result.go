package mongo

import (
	"context"
	"github.com/doubletrey/crawlab-db/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

type FindResultInterface interface {
	One(val interface{}) (err error)
	All(val interface{}) (err error)
}

func NewFindResult() (fr *FindResult) {
	return &FindResult{}
}

func NewFindResultWithError(err error) (fr *FindResult) {
	return &FindResult{
		err: err,
	}
}

type FindResult struct {
	col *Col
	res *mongo.SingleResult
	cur *mongo.Cursor
	err error
}

func (fr *FindResult) One(val interface{}) (err error) {
	if fr.err != nil {
		return fr.err
	}
	if fr.cur != nil {
		if !fr.cur.TryNext(fr.col.ctx) {
			return mongo.ErrNoDocuments
		}
		return fr.cur.Decode(val)
	}
	return fr.res.Decode(val)
}

func (fr *FindResult) All(val interface{}) (err error) {
	if fr.err != nil {
		return fr.err
	}
	var ctx context.Context
	if fr.col == nil {
		ctx = context.Background()
	} else {
		ctx = fr.col.ctx
	}
	if fr.cur == nil {
		return errors.ErrNoCursor
	}
	if !fr.cur.TryNext(ctx) {
		return ctx.Err()
	}
	return fr.cur.All(ctx, val)
}
