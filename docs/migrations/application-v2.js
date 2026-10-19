// Mongo migration: `application` collection v1 -> v2 (app-update rework).
//
// v1: {platform, version, url, text, force, latest, hot, create_time}, unique (platform, version), index latest
// v2: adds arch/build/size/sha256/package_type/min_version/status, drops latest/hot,
//     unique (platform, arch, version, build) named "uniq_release".
// arch is normalized to the canonical set used by the clients: universal|arm64|armv7|x64|x86
// (armeabi-v7a/armeabi/armv7l/armhf/arm -> armv7, amd64/x86_64/x86-64 -> x64, aarch64/arm64-v8a -> arm64,
//  i386/i686/ia32 -> x86, empty -> universal). Same map as pkg/common/appversion.NormalizeArch.
// Safe to re-run.
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
  // normalize arch spellings; a record whose canonical key already exists is reported, not changed
  const archAliases = {
    "arm64-v8a": "arm64", "aarch64": "arm64", "armv8": "arm64", "arm64e": "arm64",
    "armeabi-v7a": "armv7", "armeabi": "armv7", "armv7l": "armv7", "armv7a": "armv7", "armhf": "armv7", "arm": "armv7",
    "amd64": "x64", "x86_64": "x64", "x86-64": "x64", "x8664": "x64",
    "i386": "x86", "i686": "x86", "ia32": "x86", "386": "x86",
    "all": "universal", "any": "universal", "noarch": "universal", "": "universal",
  };
  const canonical = { universal: 1, arm64: 1, armv7: 1, x64: 1, x86: 1 };
  // legal platform/arch combinations (same table as pkg/common/appversion platformArches)
  const platformArches = {
    android: ["universal", "arm64", "armv7", "x64", "x86"],
    windows: ["universal", "x64", "arm64", "x86"],
    macos: ["universal", "arm64", "x64"],
    linux: ["universal", "x64", "arm64", "armv7"],
    ios: ["universal"],
  };
  let badCombo = 0;
  let archFixed = 0;
  c.find({}).forEach(function (d) {
    const cur = (d.arch === undefined || d.arch === null) ? "" : String(d.arch);
    let want = cur.trim().toLowerCase();
    if (archAliases.hasOwnProperty(want)) want = archAliases[want];
    if (!canonical[want]) { print("WARN unknown arch, fix or delete by hand: " + d._id + " arch=" + cur); return; }
    if (!platformArches[d.platform] || platformArches[d.platform].indexOf(want) < 0) {
      badCombo++;
      print("WARN illegal platform/arch " + d.platform + "/" + want + " (allowed: " + (platformArches[d.platform] || []).join("|") +
        "); not served for that arch, update_version will be rejected; delete and re-add: " + d._id);
    }
    if (want === cur) return;
    const clash = c.findOne({ _id: { $ne: d._id }, platform: d.platform, arch: want, version: d.version, build: d.build });
    if (clash) {
      print("WARN arch " + cur + " -> " + want + " would duplicate " + clash._id + "; keep one and delete the other: " + d._id);
      return;
    }
    c.updateOne({ _id: d._id }, { $set: { arch: want } });
    archFixed++;
  });
  print("arch normalized: " + archFixed + ", illegal platform/arch combinations: " + badCombo);
  // v1 "latest:false" rows are history only; the server now derives "latest" from (version, build).
  c.createIndex({ platform: 1, arch: 1, version: 1, build: 1 }, { unique: true, name: "uniq_release" });
  c.createIndex({ platform: 1, status: 1 }, { name: "platform_status" });
}
