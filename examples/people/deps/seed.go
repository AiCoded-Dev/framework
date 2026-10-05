package deps

// seed holds the users the table starts with. alice and bob match the personas of the examples.
var seed = []User{
	{
		Login: "johndoe123", Name: "John Doe", Age: 28, Team: "IT", Languages: "English, Spanish",
		OnSite: true, Remote: true, Shift: 1,
		Bio: "John is a highly skilled software engineer with a strong background in full-stack development. Over the last 5 years, he has worked on numerous projects, mastering multiple programming languages including Golang, Python, and JavaScript. His expertise in cloud architecture and microservices has contributed to several successful deployments, making him a key player in the company's growth.",
	},
	{
		Login: "janesmith456", Name: "Jane Smith", Age: 34, Team: "IT", Languages: "English, French",
		Remote: true, Shift: 1,
		Bio: "Jane is a seasoned product manager with over 8 years of experience in leading cross-functional teams to deliver high-quality products. She has a proven track record of managing complex projects from inception to launch, specializing in mobile and web applications. Jane is adept at utilizing agile methodologies to streamline workflows and boost productivity within her team.",
	},
	{
		Login: "alexjohnson789", Name: "Alex Johnson", Age: 26, Team: "Finance", Languages: "English, German",
		OnSite: true, Shift: 1,
		Bio: "Alex is an enthusiastic data scientist with a passion for machine learning and artificial intelligence. He has a deep understanding of statistical modeling and data analytics, and has applied his knowledge to solve complex problems in various industries. His recent work in natural language processing and predictive analytics has significantly improved customer insights for the company.",
	},
	{
		Login: "michaelbaker321", Name: "Michael Baker", Age: 45, Team: "People", Languages: "English",
		OnSite: true, Shift: 1,
		Bio: "Michael is an experienced HR manager with a career spanning over 15 years in human resources. He has successfully handled large teams and implemented effective recruitment and retention strategies. His expertise lies in fostering a healthy work environment and addressing employee grievances, ensuring that both the company's needs and employee satisfaction are well-balanced.",
	},
	{
		Login: "emilyclark654", Name: "Emily Clark", Age: 30, Team: "Marketing", Languages: "English, Portuguese, Spanish",
		OnSite: true, Shift: 2,
		Bio: "Emily is a dynamic marketing specialist with a focus on digital marketing strategies. She has successfully led campaigns that increased brand awareness and user engagement for several high-profile clients. Emily is particularly skilled at leveraging social media platforms and SEO techniques to drive organic growth and improve lead generation for businesses.",
	},
	{
		Login: "oliverjames987", Name: "Oliver James", Age: 39, Team: "IT", Languages: "English",
		OnSite: true, Remote: true, Shift: 2,
		Bio: "Oliver is a DevOps engineer with a decade of experience in automating and streamlining development operations. He is well-versed in CI/CD pipelines, cloud infrastructure, and containerization tools like Docker and Kubernetes. Oliver has led efforts to implement scalable solutions that enhance operational efficiency and reduce downtime across the development lifecycle.",
	},
	{
		Login: "sophiataylor123", Name: "Sophia Taylor", Age: 24, Team: "IT", Languages: "English, Portuguese",
		OnSite: true, Shift: 1,
		Bio: "Sophia is a junior frontend developer with a passion for creating aesthetically pleasing and highly functional user interfaces. Her attention to detail and understanding of user-centered design principles make her a valuable asset to the team. Sophia has quickly mastered several frontend technologies, including React and Vue.js, and is eager to continue honing her skills in web development.",
	},
	{
		Login: "jackwilliams456", Name: "Jack Williams", Age: 31, Team: "Marketing", Languages: "English",
		Remote: true, Shift: 1,
		Bio: "Jack is a project manager with expertise in Agile methodologies. He has successfully led numerous high-stakes projects, ensuring they are delivered on time and within budget. His excellent communication and leadership skills allow him to coordinate effectively with different departments, ensuring smooth collaboration and achieving project goals.",
	},
	{
		Login: "isabellajohnson789", Name: "Isabella Johnson", Age: 29, Team: "Marketing", Languages: "English, French, Spanish",
		OnSite: true, Shift: 2,
		Bio: "Isabella is a UI/UX designer with a deep understanding of user experience and interface design. She has worked on multiple high-profile projects, where her designs have significantly enhanced the user journey and interface usability. Her approach combines creativity with data-driven insights, ensuring that the end product not only looks great but also functions seamlessly.",
	},
	{
		Login: "davidbrown321", Name: "David Brown", Age: 42, Team: "IT", Languages: "English, German",
		OnSite: true, Remote: true, Shift: 1,
		Bio: "David is an IT consultant with over 15 years of experience in helping businesses adopt and optimize cloud computing solutions. He specializes in cloud infrastructure, cybersecurity, and data management. His strategic insights have led to significant cost savings and improved operational efficiency for his clients.",
	},
	{
		Login: "alice", Name: "Alice Moreau", Age: 36, Team: "Front desk", Languages: "English, French, Portuguese",
		OnSite: true, Shift: 1,
		Bio: "Alice runs the front desk of the Lisbon hotel and trains new receptionists.",
	},
	{
		Login: "bob", Name: "Bob Okafor", Age: 52, Team: "People", Languages: "English",
		OnSite: true, Remote: true, Shift: 1,
		Bio: "Bob leads people and HR, and adds new colleagues to the directory.",
	},
}
