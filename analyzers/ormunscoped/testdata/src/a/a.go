package a

type DB struct{}

func (d *DB) Model(v interface{}) *DB                     { return d }
func (d *DB) Where(query string, args ...interface{}) *DB { return d }
func (d *DB) Save(v interface{}) *DB                      { return d }

func unscopedViaModel(db *DB, u interface{}) {
	db.Model(u).Save(u) // want "unscoped .Save\\(\\) call: no .Where\\(\\) earlier in the chain, this can update unintended rows"
}

func directSave(db *DB, u interface{}) {
	db.Save(u) // want "unscoped .Save\\(\\) call: no .Where\\(\\) earlier in the chain, this can update unintended rows"
}

func scoped(db *DB, u interface{}) {
	db.Where("id = ?", 1).Save(u) // ok: .Where() earlier in the chain
}

func scopedViaModel(db *DB, u interface{}) {
	db.Model(u).Where("id = ?", 1).Save(u) // ok: .Where() earlier in the chain
}
