# E-GEN-049: default page is not a page below the layout

`<ssr:content default="…"/>` names the page that opening the layout shows. The default is the
path of a page below the layout, relative to it: in `pages/admin/index.html`,
`default="settings"` sends the viewer from `/admin` to `/admin/settings`, and a leading `/`
changes nothing, so `default="/settings"` does the same. It names no page below the layout, it
names the layout itself or a page elsewhere (as `..` does), or it names a parameter folder such
as `n_id`: the value of a parameter folder is not known when the pages are generated, so a
default can only name pages whose folders all have fixed names. Without this check the layout
would redirect the viewer to a page that answers 404. To open a page below a parameter folder,
or to choose the page per viewer, leave `default` out and return the page's path, relative to
the layout, from the data provider's `DefaultRoute`.

**Fix:** name a page below this one by its folders, such as `default="settings"`, or leave
`default` out and implement `DefaultRoute`
