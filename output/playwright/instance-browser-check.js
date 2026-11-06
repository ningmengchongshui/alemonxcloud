async page => {
 if (!(await page.getByText('125.50%', {exact:true}).isVisible())) throw Error('CPU sample missing');
 await page.getByRole('link',{name:'实例管理',exact:true}).click();
 await page.getByRole('switch',{name:'演示实例 的宿主机访问'}).waitFor();
 await page.getByRole('link',{name:'资源与状态',exact:true}).click();
 await page.getByRole('heading',{name:'资源同步状态',exact:true}).waitFor();
 await page.getByRole('link',{name:'概览',exact:true}).click();
 await page.route('**/api/instances/demo-instance/stats',route=>route.fulfill({status:502,contentType:'application/json',body:JSON.stringify({message:'采集失败'})}));
 await page.getByRole('button',{name:'刷新监控',exact:true}).click();
 await page.getByText('监控采集失败，请点击刷新重试。',{exact:false}).waitFor();
 await page.setViewportSize({width:390,height:844});
 await page.screenshot({path:'output/playwright/instance-detail-mobile.png',fullPage:true});
 const overflow = await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth);
 if(overflow) throw Error('mobile horizontal overflow');
 console.log('PASS: detail tabs, host access relocation, real counters, retry failure, mobile width');
}
