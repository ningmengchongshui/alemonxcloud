async page => {
 await page.route('**/api/instances',route=>route.fulfill({status:200,contentType:'application/json',body:'[]'}));
 await page.route('**/api/instances/demo-instance/summary',route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({instance:{id:'demo-instance',name:'自建实例测试',image:'demo',version:'latest',spec:'1 核 / 1 GB',status:'stopped',runtimeStatus:'stopped',createdAt:'2026-11-06T00:00:00Z',hostAccessAllowed:true,hostAccessApplied:false,hostAccessEnabled:false},selfHosted:true,nodeId:'demo-node'})}));
 await page.route('**/api/instances/demo-instance',route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({metadata:{id:'demo-instance',resourceVersion:1},spec:{},status:{lifecycle:'stopped',runtimeState:'stopped'},conditions:[],operations:[]})}));
 await page.goto('http://127.0.0.1:5173/me/instances/demo-instance/manage');
 await page.getByRole('switch',{name:'自建实例测试 的宿主机访问'}).waitFor();
 if(await page.getByRole('button',{name:'变更套餐',exact:true}).count()) throw Error('selfhosted plan change must stay hidden');
 if(!(await page.getByRole('button',{name:'启动',exact:true}).isVisible())) throw Error('stopped instance start action missing');
 await page.getByRole('link',{name:'概览',exact:true}).click();
 await page.getByText('实例未运行，暂无实时占用数据。',{exact:true}).waitFor();
 await page.reload();
 await page.getByRole('heading',{name:'自建实例测试',exact:true}).waitFor();
 await page.screenshot({path:'output/playwright/selfhosted-instance-mobile.png',fullPage:true});
 console.log('PASS: selfhosted direct route/reload, management controls, stopped usage');
}
