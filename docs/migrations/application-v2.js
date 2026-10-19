// Mongo migration: `application` collection v1 -> v2 (app-update rework).
//
// v1: {platform, version, url, text, force, latest, hot, create_time}, unique (platform, version), index latest
// v2: adds arch/build/size/sha256/package_type/min_version/status, drops latest/hot,
//     unique (platform, arch, version, build) named "uniq_release".
//
// NOT run automatically. The service itself creates the new indexes and drops the two v1 indexes
// on start (NewApplication), so for a fresh or already-emptied collection nothing needs doing.
//
// Usage (review first, run on a copy / test DB before any real one):
//   mongosh "mongodb://user:pass@host:27017/openim_v3?authSource=admin" docs/migrations/application-v2.js
//   # or, to start from scratch (the feature was never enabled, data is disposable):
//   mongosh ... --eval 'DROP=true' docs/migrations/application-v2.js
const c = db.getCollection("application");
if (typeof DROP !== "undefined" && DROP === true) {
  c.drop();
  print("application collection dropped; the service recreates it with v2 indexes on next start");
} else {
  ["platform_1_version_1", "latest_-1"].forEach(function (n) {
    try { c.dropIndex(n); print("dropped index " + n); } catch (e) { print("index " + n + " absent"); }
  });
  const semver = /^\d+\.\d+\.\d+$/;
  c.find({}).forEach(function (d) {
    if (!semver.test(d.version || "")) {
      print("WARN invalid semver, fix or delete by hand: " + d._id + " version=" + d.version);
    }
  });
  const r = c.updateMany(
    { arch: { $exists: false } },
    {
      $set: { arch: "universal", build: NumberLong(0), size: NumberLong(0), sha256: "", package_type: "", min_version: "", status: "published" },
      $unset: { latest: "", hot: "" },
    }
  );
  print("migrated documents: " + r.modifiedCount);
  // v1 "latest:false" rows are history only; the server now derives "latest" from (version, build).
  c.createIndex({ platform: 1, arch: 1, version: 1, build: 1 }, { unique: true, name: "uniq_release" });
  c.createIndex({ platform: 1, status: 1 }, { name: "platform_status" });
}
