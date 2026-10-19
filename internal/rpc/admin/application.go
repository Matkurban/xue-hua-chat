package admin

import (
	"chat/pkg/common/appversion"
	admindb "chat/pkg/common/db/table/admin"
	"chat/pkg/common/mctx"
	"chat/pkg/protocol/admin"
	"context"
	"gitee.com/matkurban/openimsdk-tools/errs"
	"gitee.com/matkurban/openimsdk-tools/utils/datautil"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"time"
)

func IsNotFound(err error) bool {
	switch errs.Unwrap(err) {
	case redis.Nil, mongo.ErrNoDocuments:
		return true
	default:
		return false
	}
}

func (o *adminServer) db2pbApplication(val *admindb.Application) *admin.ApplicationVersion {
	return &admin.ApplicationVersion{
		Id:          val.ID.Hex(),
		Platform:    val.Platform,
		Arch:        appversion.NormalizeArch(val.Arch),
		Version:     val.Version,
		Build:       val.Build,
		Url:         val.Url,
		Size:        val.Size,
		Sha256:      val.Sha256,
		PackageType: val.PackageType,
		Text:        val.Text,
		MinVersion:  val.MinVersion,
		Force:       val.Force,
		Status:      val.Status,
		CreateTime:  val.CreateTime.UnixMilli(),
	}
}

// LatestApplicationVersion is called by clients (no auth, it must work from the login screen).
// The comparison, force and min_version rules run on the server; see appversion.Check.
func (o *adminServer) LatestApplicationVersion(ctx context.Context, req *admin.LatestApplicationVersionReq) (*admin.LatestApplicationVersionResp, error) {
	if _, err := appversion.ParseVersion(req.Version); err != nil {
		return nil, errs.ErrArgs.WrapMsg("version: " + err.Error())
	}
	if req.Platform == "" {
		return nil, errs.ErrArgs.WrapMsg("platform is empty")
	}
	if req.Build < 0 {
		return nil, errs.ErrArgs.WrapMsg("build must be >= 0")
	}
	list, err := o.Database.ListPublishedVersions(ctx, req.Platform)
	if err != nil && !IsNotFound(err) {
		return nil, err
	}
	res, err := appversion.Check(list, req.Version, req.Build, req.Arch)
	if err != nil {
		return nil, errs.ErrArgs.WrapMsg(err.Error())
	}
	resp := &admin.LatestApplicationVersionResp{HasUpdate: res.HasUpdate, Force: res.Force}
	if res.Latest != nil {
		resp.Latest = o.db2pbApplication(res.Latest)
	}
	return resp, nil
}

func (o *adminServer) AddApplicationVersion(ctx context.Context, req *admin.AddApplicationVersionReq) (*admin.AddApplicationVersionResp, error) {
	if _, err := mctx.CheckAdmin(ctx); err != nil {
		return nil, err
	}
	val := &admindb.Application{
		ID:          bson.NewObjectID(),
		Platform:    req.Platform,
		Arch:        req.Arch,
		Version:     req.Version,
		Build:       req.Build,
		Url:         req.Url,
		Size:        req.Size,
		Sha256:      req.Sha256,
		PackageType: req.PackageType,
		Text:        req.Text,
		Force:       req.Force,
		MinVersion:  req.MinVersion,
		Status:      req.Status,
		CreateTime:  time.Now(),
	}
	if err := appversion.Validate(val); err != nil {
		return nil, errs.ErrArgs.WrapMsg(err.Error())
	}
	if err := o.Database.AddVersion(ctx, val); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, errs.ErrDuplicateKey.WrapMsg("this platform/arch/version/build already exists")
		}
		return nil, err
	}
	return &admin.AddApplicationVersionResp{Id: val.ID.Hex()}, nil
}

// UpdateApplicationVersion edits the mutable fields only; platform/arch/version/build identify the
// release and cannot change (they are not even part of the request).
func (o *adminServer) UpdateApplicationVersion(ctx context.Context, req *admin.UpdateApplicationVersionReq) (*admin.UpdateApplicationVersionResp, error) {
	if _, err := mctx.CheckAdmin(ctx); err != nil {
		return nil, err
	}
	oid, err := bson.ObjectIDFromHex(req.Id)
	if err != nil {
		return nil, errs.ErrArgs.WrapMsg("invalid id " + err.Error())
	}
	cur, err := o.Database.TakeVersion(ctx, oid)
	if err != nil {
		if IsNotFound(err) {
			return nil, errs.ErrRecordNotFound.WrapMsg("version not found")
		}
		return nil, err
	}
	merged := *cur
	if v := req.Url; v != nil {
		merged.Url = v.Value
	}
	if v := req.Size; v != nil {
		merged.Size = v.Value
	}
	if v := req.Sha256; v != nil {
		merged.Sha256 = v.Value
	}
	if v := req.PackageType; v != nil {
		merged.PackageType = v.Value
	}
	if v := req.Text; v != nil {
		merged.Text = v.Value
	}
	if v := req.MinVersion; v != nil {
		merged.MinVersion = v.Value
	}
	if v := req.Force; v != nil {
		merged.Force = v.Value
	}
	if v := req.Status; v != nil {
		merged.Status = v.Value
	}
	if req.Url == nil && req.Size == nil && req.Sha256 == nil && req.PackageType == nil && req.Text == nil &&
		req.MinVersion == nil && req.Force == nil && req.Status == nil {
		return nil, errs.ErrArgs.WrapMsg("no update info")
	}
	if err := appversion.Validate(&merged); err != nil {
		return nil, errs.ErrArgs.WrapMsg(err.Error())
	}
	update := map[string]any{
		"url": merged.Url, "size": merged.Size, "sha256": merged.Sha256, "package_type": merged.PackageType,
		"text": merged.Text, "min_version": merged.MinVersion, "force": merged.Force, "status": merged.Status,
	}
	if merged.Arch != cur.Arch {
		// legacy alias spelling (e.g. armeabi-v7a/amd64): store the canonical name on the next edit
		update["arch"] = merged.Arch
	}
	if err := o.Database.UpdateVersion(ctx, oid, update); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, errs.ErrDuplicateKey.WrapMsg("a record with the canonical arch " + merged.Arch + " already exists for this platform/version/build")
		}
		return nil, err
	}
	return &admin.UpdateApplicationVersionResp{}, nil
}

// RevokeApplicationVersion withdraws a release: it stops being served to clients but is kept for history.
func (o *adminServer) RevokeApplicationVersion(ctx context.Context, req *admin.RevokeApplicationVersionReq) (*admin.RevokeApplicationVersionResp, error) {
	if _, err := mctx.CheckAdmin(ctx); err != nil {
		return nil, err
	}
	oid, err := bson.ObjectIDFromHex(req.Id)
	if err != nil {
		return nil, errs.ErrArgs.WrapMsg("invalid id " + err.Error())
	}
	if _, err := o.Database.TakeVersion(ctx, oid); err != nil {
		if IsNotFound(err) {
			return nil, errs.ErrRecordNotFound.WrapMsg("version not found")
		}
		return nil, err
	}
	if err := o.Database.UpdateVersion(ctx, oid, map[string]any{"status": appversion.StatusRevoked}); err != nil {
		return nil, err
	}
	return &admin.RevokeApplicationVersionResp{}, nil
}

func (o *adminServer) DeleteApplicationVersion(ctx context.Context, req *admin.DeleteApplicationVersionReq) (*admin.DeleteApplicationVersionResp, error) {
	if _, err := mctx.CheckAdmin(ctx); err != nil {
		return nil, err
	}
	ids := make([]bson.ObjectID, 0, len(req.Id))
	for _, id := range req.Id {
		oid, err := bson.ObjectIDFromHex(id)
		if err != nil {
			return nil, errs.ErrArgs.WrapMsg("invalid id " + err.Error())
		}
		ids = append(ids, oid)
	}
	if err := o.Database.DeleteVersion(ctx, ids); err != nil {
		return nil, err
	}
	return &admin.DeleteApplicationVersionResp{}, nil
}

func (o *adminServer) PageApplicationVersion(ctx context.Context, req *admin.PageApplicationVersionReq) (*admin.PageApplicationVersionResp, error) {
	if _, err := mctx.CheckAdmin(ctx); err != nil {
		return nil, err
	}
	total, res, err := o.Database.PageVersion(ctx, req.Platform, req.Pagination)
	if err != nil {
		return nil, err
	}
	return &admin.PageApplicationVersionResp{
		Total:    total,
		Versions: datautil.Slice(res, o.db2pbApplication),
	}, nil
}
