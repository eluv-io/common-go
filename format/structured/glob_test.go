package structured_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/eluv-io/log-go"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/util/jsonutil"
)

func TestFilterGlob(t *testing.T) {
	log.Get("/").SetDebug()

	tc := newCtx(t)

	type msi = map[string]interface{}
	type args struct {
		target      interface{}   // if single target
		targets     []interface{} // if multiple targets
		selectPaths []structured.Path
		removePaths []structured.Path
	}
	tests := []struct {
		name  string
		args  args
		want  interface{}
		wants []interface{} // as many as in targets
	}{
		{
			name: "basic",
			args: args{
				target: tc.parse(`
							{
							  "a": {
								"b": "bbb",
								"c": "ccc",
								"d": [
								  {
									"e": "eee",
									"f": "fff"
								  },
								  {
									"e": "eee",
									"g": "ggg"
								  }
								]
							  }
							}
						`),
				selectPaths: []structured.Path{
					structured.ParsePath("/a/b"),
					structured.ParsePath("/a/d"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/a/d/*/e"),
				},
			},
			want: tc.parse(`
							{
							  "a": {
								"b": "bbb",
								"d": [
								  {
									"f": "fff"
								  },
								  {
									"g": "ggg"
								  }
								]
							  }
							}
							`),
		},
		{
			name: "t1-no-sr",
			args: args{
				targets:     []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: nil,
				removePaths: nil,
			},
			wants: []interface{}{tc.site(), tc.siteWithArrays()},
		},
		{
			name: "t1.1-select-root",
			args: args{
				targets:     []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: []structured.Path{{}}, // "/"
				removePaths: nil,
			},
			wants: []interface{}{tc.site(), tc.siteWithArrays()},
		},
		{
			name: "t1.1.1-remove-root",
			args: args{
				targets:     []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: nil,
				removePaths: []structured.Path{{}}, // "/"
			},
			want: nil,
		},
		{
			name: "t1.2-select-non-exist",
			args: args{
				targets: []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: []structured.Path{
					structured.ParsePath("/public/not-exist"),
				},
			},
			want: nil,
		},
		{
			name: "t1.3-sr-non-exist",
			args: args{
				targets: []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: []structured.Path{
					structured.ParsePath("/public/not-exist"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/*/*/not-exist2"),
				},
			},
			want: nil,
		},
		{
			name: "t1.4",
			args: args{
				target: tc.site(),
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/*/*/not-exist2"),
				},
			},
			want: tc.site(),
		},
		{
			name: "t1.4.arrays",
			args: args{
				target: tc.siteWithArrays(),
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/*/*/not-exist2"),
				},
			},
			want: tc.siteWithArrays(),
		},
		{
			name: "t2.1",
			args: args{
				targets: []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: []structured.Path{
					structured.ParsePath("/public/name"),
				},
			},
			want: msi{
				"public": msi{
					"name": "Test Site",
				},
			},
		},
		{
			name: "t2.2",
			args: args{
				targets: []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: []structured.Path{
					structured.ParsePath("/public/name"),
					structured.ParsePath("/public/description"),
				},
			},
			want: msi{
				"public": msi{
					"name":        "Test Site",
					"description": "A beautiful sight",
				},
			},
		},
		{
			name: "t2.3",
			args: args{
				targets: []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: []structured.Path{
					structured.ParsePath("/public"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata"),
				},
			},
			want: msi{
				"public": msi{
					"name":        "Test Site",
					"description": "A beautiful sight",
				},
			},
		},
		{
			name: "t2.4",
			args: args{
				targets: []interface{}{tc.site(), tc.siteWithArrays()},
				selectPaths: []structured.Path{
					structured.ParsePath("/public"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles"),
				},
			},
			want: msi{
				"public": msi{
					"name":        "Test Site",
					"description": "A beautiful sight",
					"asset_metadata": msi{
						"asset_type": "primary",
						"title_type": "franchise",
					},
				},
			},
		},
		{
			name: "t3",
			args: args{
				target: tc.site(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/1/*/title"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "titles": {
									"1": {
									  "Slug-1": {
										"title": "Title 1"
									  }
									}
								  }
								}
							  }
							}
					`),
		},
		{
			name: "t3.arrays",
			args: args{
				target: tc.siteWithArrays(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/1/*/title"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "titles": [
									{
									  "Slug-1": {
										"title": "Title 1"
									  }
									}
								  ]
								}
							  }
							}
					`),
		},
		{
			name: "t3.1",
			args: args{
				target: tc.site(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/title"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "titles": {
									"0": {
									  "Slug-0": {
										"title": "Title 0"
									  }
									},
									"1": {
									  "Slug-1": {
										"title": "Title 1"
									  }
									},
									"2": {
									  "Slug-2": {
										"title": "Title 2"
									  }
									}
								  }
								}
							  }
							}
					`),
		},
		{
			name: "t3.arrays",
			args: args{
				target: tc.siteWithArrays(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/title"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "titles": [
									{
									  "Slug-0": {
										"title": "Title 0"
									  }
									},
									{
									  "Slug-1": {
										"title": "Title 1"
									  }
									},
									{
									  "Slug-2": {
										"title": "Title 2"
									  }
									}
								  ]
								}
							  }
							}
					`),
		},
		{
			name: "t4",
			args: args{
				target: tc.site(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/assets"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "asset_type": "primary",
								  "title_type": "franchise",
								  "titles": {
									"0": {
									  "Slug-0": {
										"asset_type": "primary",
										"title": "Title 0",
										"title_type": "feature",
										"slug": "Slug-0"
									  }
									},
									"1": {
									  "Slug-1": {
										"asset_type": "primary",
										"title": "Title 1",
										"title_type": "feature",
										"slug": "Slug-1"
									  }
									},
									"2": {
									  "Slug-2": {
										"asset_type": "primary",
										"title": "Title 2",
										"title_type": "feature",
										"slug": "Slug-2"
									  }
									}
								  }
								}
							  }
							}
						`),
		},
		{
			name: "t4.arrays",
			args: args{
				target: tc.siteWithArrays(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/assets"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "asset_type": "primary",
								  "title_type": "franchise",
								  "titles": [
									{
									  "Slug-0": {
										"asset_type": "primary",
										"title": "Title 0",
										"title_type": "feature",
										"slug": "Slug-0"
									  }
									},
									{
									  "Slug-1": {
										"asset_type": "primary",
										"title": "Title 1",
										"title_type": "feature",
										"slug": "Slug-1"
									  }
									},
									{
									  "Slug-2": {
										"asset_type": "primary",
										"title": "Title 2",
										"title_type": "feature",
										"slug": "Slug-2"
									  }
									}
								  ]
								}
							  }
							}
						`),
		},
		{
			name: "t5",
			args: args{
				target: tc.site(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/*"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/assets"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "asset_type": "primary",
								  "title_type": "franchise",
								  "titles": {
									"0": {
									  "Slug-0": {
										"asset_type": "primary",
										"title": "Title 0",
										"title_type": "feature",
										"slug": "Slug-0"
									  }
									},
									"1": {
									  "Slug-1": {
										"asset_type": "primary",
										"title": "Title 1",
										"title_type": "feature",
										"slug": "Slug-1"
									  }
									},
									"2": {
									  "Slug-2": {
										"asset_type": "primary",
										"title": "Title 2",
										"title_type": "feature",
										"slug": "Slug-2"
									  }
									}
								  }
								}
							  }
							}
						`),
		},
		{
			name: "t5.arrays",
			args: args{
				target: tc.siteWithArrays(),
				selectPaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/*"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/assets"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
								"asset_metadata": {
								  "asset_type": "primary",
								  "title_type": "franchise",
								  "titles": [
									{
									  "Slug-0": {
										"asset_type": "primary",
										"title": "Title 0",
										"title_type": "feature",
										"slug": "Slug-0"
									  }
									},
									{
									  "Slug-1": {
										"asset_type": "primary",
										"title": "Title 1",
										"title_type": "feature",
										"slug": "Slug-1"
									  }
									},
									{
									  "Slug-2": {
										"asset_type": "primary",
										"title": "Title 2",
										"title_type": "feature",
										"slug": "Slug-2"
									  }
									}
								  ]
								}
							  }
							}
						`),
		},
		{
			name: "t6",
			args: args{
				target: tc.site(),
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/assets"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
							    "name": "Test Site",
							    "description": "A beautiful sight",
								"asset_metadata": {
								  "asset_type": "primary",
								  "title_type": "franchise",
								  "titles": {
									"0": {
									  "Slug-0": {
										"asset_type": "primary",
										"title": "Title 0",
										"title_type": "feature",
										"slug": "Slug-0"
									  }
									},
									"1": {
									  "Slug-1": {
										"asset_type": "primary",
										"title": "Title 1",
										"title_type": "feature",
										"slug": "Slug-1"
									  }
									},
									"2": {
									  "Slug-2": {
										"asset_type": "primary",
										"title": "Title 2",
										"title_type": "feature",
										"slug": "Slug-2"
									  }
									}
								  }
								}
							  }
							}
						`),
		},
		{
			name: "t6.arrays",
			args: args{
				target: tc.siteWithArrays(),
				removePaths: []structured.Path{
					structured.ParsePath("/public/asset_metadata/titles/*/*/assets"),
				},
			},
			want: tc.parse(`
							{
							  "public": {
							    "name": "Test Site",
							    "description": "A beautiful sight",
								"asset_metadata": {
								  "asset_type": "primary",
								  "title_type": "franchise",
								  "titles": [
									{
									  "Slug-0": {
										"asset_type": "primary",
										"title": "Title 0",
										"title_type": "feature",
										"slug": "Slug-0"
									  }
									},
									{
									  "Slug-1": {
										"asset_type": "primary",
										"title": "Title 1",
										"title_type": "feature",
										"slug": "Slug-1"
									  }
									},
									{
									  "Slug-2": {
										"asset_type": "primary",
										"title": "Title 2",
										"title_type": "feature",
										"slug": "Slug-2"
									  }
									}
								  ]
								}
							  }
							}
						`),
		},
		{
			name: "iss-1237",
			args: args{
				target: tc.iss1237(),
				selectPaths: []structured.Path{
					structured.ParsePath("*/*/title"),
					structured.ParsePath("*/title"),
					structured.ParsePath("*/."),
				},
			},
			want: tc.parse(`
							{
							  "iron-sky---the-coming-race": {
								".": {
								  "source": "hq__TDWrPeHmExp5roZ2UwcBnQ7KfVV73ho1nLJ8ZkKTU5TjetbwPBSvKwn4wUNYxscyWEJPLYceF"
								},
								"title": "Iron Sky - The Coming Race (2019)"
							  },
							  "meridian": {
								".": {
								  "source": "hq__BkabPUUnrySh3aH2dFxqc6K34FRfU1q4odFERifoPzzBtMP1AepTLqB9MBQCKjDp77K4ounSyi"
								},
								"title": "Meridian"
							  }
							}
					`),
		},
		{
			name: "iss-1237.arrays.1",
			args: args{
				target: tc.parse(`
									[
									  [
										{
										  "title": "title 1",
										  "a": "va",
										  "b": "vb"
										}
									  ],
									  [
										{
										  "title": "title 2",
										  "c": "vc",
										  "d": "vd"
										}
									  ]
									]

								`),
				selectPaths: []structured.Path{
					structured.ParsePath("*/*/title"),
					structured.ParsePath("*/*/*/title"),
					structured.ParsePath("*/title"),
					structured.ParsePath("*/."),
					structured.ParsePath("*/*/path/does/not/exist"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("*/*/whatever"),
				},
			},
			want: tc.parse(`
							[
							  [
								{
								  "title": "title 1"
								}
							  ],
							  [
								{
								  "title": "title 2"
								}
							  ]
							]
					`),
		},
		{
			name: "iss-1237.arrays.2",
			args: args{
				target: tc.parse(`
									{
									  "m1": [
										{
										  "title": "title 1",
										  "a": "va",
										  "b": "vb"
										}
									  ],
									  "m2": [
										{
										  "title": "title 2",
										  "c": "vc",
										  "d": "vd"
										}
									  ]
									}

								`),
				selectPaths: []structured.Path{
					structured.ParsePath("*/*/title"),
					structured.ParsePath("*/*/*/title"),
					structured.ParsePath("*/title"),
					structured.ParsePath("*/."),
					structured.ParsePath("*/*/path/does/not/exist"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("*/*/whatever"),
				},
			},
			want: tc.parse(`
							{
							  "m1": [
								{
								  "title": "title 1"
								}
							  ],
							  "m2": [
								{
								  "title": "title 2"
								}
							  ]
							}
					`),
		},
		{
			name: "iss-1237.arrays.3",
			args: args{
				target: tc.parse(`
									[
									  {
									    "title": "title 1",
									    "a": "va",
									    "b": "vb"
									  },
									  {
									    "title": "title 2",
									    "c": "vc",
									    "d": "vd"
									  }
									]
								`),
				selectPaths: []structured.Path{
					structured.ParsePath("*/*/title"),
					structured.ParsePath("*/*/*/title"),
					structured.ParsePath("*/title"),
					structured.ParsePath("*/."),
					structured.ParsePath("*/*/path/does/not/exist"),
				},
				removePaths: []structured.Path{
					structured.ParsePath("*/*/whatever"),
				},
			},
			want: tc.parse(`
							[
							  {
							    "title": "title 1"
							  },
							  {
							    "title": "title 2"
							  }
							]
					`),
		},
		{
			name: "search-offerings",
			args: args{
				target: tc.searchOfferings(),
				selectPaths: []structured.Path{
					structured.ParsePath("/offerings/*/ready"),
				},
			},
			want: tc.parse(`
							{
							  "offerings": {
								"default": {
								  "ready": true
								},
								"watermark-large": {
								  "ready": true
								},
								"watermark-medium": {
								  "ready": true
								},
								"watermark-small": {
								  "ready": true
								}
							  }
							}
						`),
		},
	}
	for _, tt := range tests {
		targets := []interface{}{tt.args.target}
		if tt.args.targets != nil {
			targets = tt.args.targets
		}
		wants := make([]interface{}, len(targets))
		if tt.wants == nil {
			for i := range wants {
				wants[i] = tt.want
			}
		} else {
			wants = tt.wants
		}
		for i, target := range targets {
			name := tt.name
			if i > 0 {
				name = fmt.Sprintf("%s-target-%d", name, i+1)
			}
			t.Run(name, func(t *testing.T) {
				fmt.Println("test", name)
				got := structured.FilterGlob(target, tt.args.selectPaths, tt.args.removePaths)
				tc.Equal(jsonutil.MarshalString(wants[i]), jsonutil.MarshalString(got), jsonutil.MarshalString(got))
				tc.Equal(wants[i], got, jsonutil.MarshalString(got))
			})
		}
	}
}

type tctx struct {
	*require.Assertions
}

func newCtx(t *testing.T) *tctx {
	return &tctx{
		Assertions: require.New(t),
	}
}

func (tc *tctx) site() interface{} {
	bytes, err := ioutil.ReadFile("testdata/site.json")
	tc.NoError(err)
	return tc.parse(string(bytes))
}

func (tc *tctx) siteWithArrays() interface{} {
	bytes, err := ioutil.ReadFile("testdata/site_with_arrays.json")
	tc.NoError(err)
	return tc.parse(string(bytes))
}

func (tc *tctx) iss1237() interface{} {
	bytes, err := ioutil.ReadFile("testdata/iss-1237.json")
	tc.NoError(err)
	return tc.parse(string(bytes))
}

func (tc *tctx) searchOfferings() interface{} {
	bytes, err := ioutil.ReadFile("testdata/search-offerings.json")
	tc.NoError(err)
	return tc.parse(string(bytes))
}

func (tc *tctx) parse(s string) interface{} {
	var res interface{}
	err := json.Unmarshal([]byte(s), &res)
	tc.NoError(err)
	return res
}
