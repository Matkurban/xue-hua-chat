// User register/login time backfill (mongosh). READ-ONLY by default.
//
//   mongosh "<uri>/openim_chat" --eval 'var APPLY=false' docs/migrations/user-times-backfill.js   // report only
//   mongosh "<uri>/openim_chat" --eval 'var APPLY=true'  docs/migrations/user-times-backfill.js   // write
//
// Collections: attribute(create_time), register(create_time), user_login_record(login_time).
// (Collection names are the Mongo ones used by pkg/common/db/model/chat, NOT the TableName() values.)
//
// What it does:
//  1. attribute.create_time missing / epoch-0  ->  copy register.create_time (the API already falls back to the
//     register table at read time, so this is optional; it just makes the data self-consistent).
//  2. REPORTS users that have no user_login_record. It does NOT invent login records: they would pollute the
//     login_user_count statistics. Those users simply show an empty "last login" until their next login.
//  3. Drops the old, useless user_login_record index on create_time (field does not exist); the new
//     (user_id, login_time) index is created by the service at startup.
var apply = (typeof APPLY !== 'undefined') && APPLY === true;
print('mode: ' + (apply ? 'APPLY' : 'DRY-RUN'));

var epoch = new Date(1000);
var missing = { $or: [{ create_time: { $exists: false } }, { create_time: null }, { create_time: { $lte: epoch } }] };
var fixed = 0, noRegister = 0;
db.attribute.find(missing, { user_id: 1 }).forEach(function (a) {
  var r = db.register.findOne({ user_id: a.user_id }, { create_time: 1 });
  if (!r || !r.create_time) { noRegister++; return; }
  fixed++;
  if (apply) db.attribute.updateOne({ _id: a._id }, { $set: { create_time: r.create_time } });
});
print('attributes with empty create_time: fixable from register=' + fixed + ', no register row=' + noRegister);

var neverLoggedIn = 0;
db.attribute.find({}, { user_id: 1 }).forEach(function (a) {
  if (db.user_login_record.countDocuments({ user_id: a.user_id }, { limit: 1 }) === 0) neverLoggedIn++;
});
print('users with no login record (last login will be empty): ' + neverLoggedIn);

if (apply) {
  try { db.user_login_record.dropIndex('create_time_1'); print('dropped index create_time_1'); }
  catch (e) { print('index create_time_1 not present: ' + e.message); }
}
